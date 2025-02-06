// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/slice"
	"sigs.k8s.io/cli-utils/cmd/flagutils"
	"sigs.k8s.io/cli-utils/cmd/status/printers"
	"sigs.k8s.io/cli-utils/cmd/status/printers/printer"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/aggregator"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/collector"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
	"sigs.k8s.io/cli-utils/pkg/object"
	printcommon "sigs.k8s.io/cli-utils/pkg/print/common"
	pkgprinters "sigs.k8s.io/cli-utils/pkg/printers"
)

const (
	Known   = "known"
	Current = "current"
	Deleted = "deleted"
	Forever = "forever"
)

const (
	Local  = "local"
	Remote = "remote"
)

var (
	PollUntilOptions = []string{Known, Current, Deleted, Forever}
)

func GetRunner(ctx context.Context, factory cmdutil.Factory,
	invFactory inventory.ClientFactory, loader Loader) *Runner {
	r := &Runner{
		ctx:               ctx,
		factory:           factory,
		invFactory:        invFactory,
		loader:            loader,
		PollerFactoryFunc: pollerFactoryFunc,
	}
	c := &cobra.Command{
		Use:     "status (DIRECTORY | STDIN)",
		PreRunE: r.preRunE,
		RunE:    r.runE,
	}
	c.Flags().DurationVar(&r.period, "poll-period", 2*time.Second,
		"Polling period for resource statuses.")
	c.Flags().StringVar(&r.pollUntil, "poll-until", "known",
		"When to stop polling. Must be one of 'known', 'current', 'deleted', or 'forever'.")
	c.Flags().StringVar(&r.output, "output", "events", "Output format.")
	c.Flags().DurationVar(&r.timeout, "timeout", 0,
		"How long to wait before exiting")
	c.Flags().StringVar(&r.invType, "inv-type", Local, "Type of the inventory info, must be local or remote")
	c.Flags().StringVar(&r.inventoryNames, "inv-names", "", "Names of targeted inventory: inv1,inv2,...")
	c.Flags().StringVar(&r.namespaces, "namespaces", "", "Names of targeted namespaces: ns1,ns2,...")
	c.Flags().StringVar(&r.statuses, "statuses", "", "Targeted status: st1,st2...")

	r.Command = c
	return r
}

func Command(ctx context.Context, f cmdutil.Factory,
	invFactory inventory.ClientFactory, loader Loader) *cobra.Command {
	return GetRunner(ctx, f, invFactory, loader).Command
}

// Runner captures the parameters for the command and contains
// the run function.
type Runner struct {
	ctx        context.Context
	Command    *cobra.Command
	factory    cmdutil.Factory
	invFactory inventory.ClientFactory
	loader     Loader

	period    time.Duration
	pollUntil string
	timeout   time.Duration
	output    string

	invType          string
	inventoryNames   string
	inventoryNameSet map[string]bool
	namespaces       string
	namespaceSet     map[string]bool
	statuses         string
	statusSet        map[string]bool

	PollerFactoryFunc func(cmdutil.Factory) (poller.Poller, error)
}

func (r *Runner) preRunE(*cobra.Command, []string) error {
	if !slice.ContainsString(PollUntilOptions, r.pollUntil, nil) {
		return fmt.Errorf("pollUntil must be one of %s", strings.Join(PollUntilOptions, ","))
	}

	if found := pkgprinters.ValidatePrinterType(r.output); !found {
		return fmt.Errorf("unknown output type %q", r.output)
	}

	if r.invType != Local && r.invType != Remote {
		return fmt.Errorf("inv-type flag should be either local or remote")
	}

	if r.invType == Local && r.inventoryNames != "" {
		return fmt.Errorf("inv-names flag should only be used when inv-type is set to remote")
	}

	if r.inventoryNames != "" {
		r.inventoryNameSet = make(map[string]bool)
		for _, name := range strings.Split(r.inventoryNames, ",") {
			r.inventoryNameSet[name] = true
		}
	}

	if r.namespaces != "" {
		r.namespaceSet = make(map[string]bool)
		for _, ns := range strings.Split(r.namespaces, ",") {
			r.namespaceSet[ns] = true
		}
	}

	if r.statuses != "" {
		r.statusSet = make(map[string]bool)
		for _, st := range strings.Split(r.statuses, ",") {
			parsedST := strings.ToLower(st)
			r.statusSet[parsedST] = true
		}
	}

	return nil
}

// Load inventory info from local storage
// and get info from the cluster based on the local info
// wrap it to be a map mapping from string to objectMetadataSet
func (r *Runner) loadInvFromDisk(cmd *cobra.Command, args []string) (*printer.PrintData, error) {
	inv, err := r.loader.GetInvInfo(cmd, args)
	if err != nil {
		return nil, err
	}

	invClient, err := r.invFactory.NewClient(r.factory)
	if err != nil {
		return nil, err
	}

	// Based on the inventory template manifest we look up the inventory
	// from the live state using the inventory client.
	clusterInventory, err := invClient.Get(cmd.Context(), inv, inventory.GetOptions{})
	if err != nil {
		return nil, err
	}

	identifiers := clusterInventory.Objects()

	printData := printer.PrintData{
		Identifiers: object.ObjMetadataSet{},
		InvNameMap:  make(map[object.ObjMetadata]string),
		StatusSet:   r.statusSet,
	}

	for _, obj := range identifiers {
		// check if the object is under one of the targeted namespaces
		if _, ok := r.namespaceSet[obj.Namespace]; ok || len(r.namespaceSet) == 0 {
			// add to the map for future reference
			printData.InvNameMap[obj] = inv.Name()
			// append to identifiers
			printData.Identifiers = append(printData.Identifiers, obj)
		}
	}
	return &printData, nil
}

// Retrieve a list of inventory object from the cluster
func (r *Runner) listInvFromCluster() (*printer.PrintData, error) {
	invClient, err := r.invFactory.NewClient(r.factory)
	if err != nil {
		return nil, err
	}

	// initialize maps in printData
	printData := printer.PrintData{
		Identifiers: object.ObjMetadataSet{},
		InvNameMap:  make(map[object.ObjMetadata]string),
		StatusSet:   r.statusSet,
	}

	inventories, err := invClient.List(r.ctx, inventory.ListOptions{})
	if err != nil {
		return nil, err
	}

	identifiersMap := make(map[string]object.ObjMetadataSet)
	for _, inv := range inventories {
		identifiersMap[inv.ID()] = inv.Objects()
	}

	for invName, identifiers := range identifiersMap {
		// Check if there are targeted inventory names and include the current inventory name
		if _, ok := r.inventoryNameSet[invName]; !ok && len(r.inventoryNameSet) != 0 {
			continue
		}
		// Filter objects
		for _, obj := range identifiers {
			// check if the object is under one of the targeted namespaces
			if _, ok := r.namespaceSet[obj.Namespace]; ok || len(r.namespaceSet) == 0 {
				// add to the map for future reference
				printData.InvNameMap[obj] = invName
				// append to identifiers
				printData.Identifiers = append(printData.Identifiers, obj)
			}
		}
	}
	return &printData, nil
}

// runE implements the logic of the command and will delegate to the
// poller to compute status for each of the resources. One of the printer
// implementations takes care of printing the output.
func (r *Runner) runE(cmd *cobra.Command, args []string) error {
	var printData *printer.PrintData
	var err error
	switch r.invType {
	case Local:
		if len(args) != 0 {
			printcommon.SprintfWithColor(printcommon.YELLOW,
				"Warning: Path is assigned while list flag is enabled, ignore the path")
		}
		printData, err = r.loadInvFromDisk(cmd, args)
	case Remote:
		printData, err = r.listInvFromCluster()
	default:
		return fmt.Errorf("invType must be either local or remote")
	}
	if err != nil {
		return err
	}

	// Exit here if the inventory is empty.
	if len(printData.Identifiers) == 0 {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "no resources found in the inventory\n")
		return nil
	}

	statusPoller, err := r.PollerFactoryFunc(r.factory)
	if err != nil {
		return err
	}

	// Fetch a printer implementation based on the desired output format as
	// specified in the output flag.
	printer, err := printers.CreatePrinter(r.output, genericiooptions.IOStreams{
		In:     cmd.InOrStdin(),
		Out:    cmd.OutOrStdout(),
		ErrOut: cmd.ErrOrStderr(),
	}, printData)
	if err != nil {
		return fmt.Errorf("error creating printer: %w", err)
	}

	// If the user has specified a timeout, we create a context with timeout,
	// otherwise we create a context with cancel.
	ctx := cmd.Context()
	var cancel func()
	if r.timeout != 0 {
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// Choose the appropriate ObserverFunc based on the criteria for when
	// the command should exit.
	var cancelFunc collector.ObserverFunc
	switch r.pollUntil {
	case "known":
		cancelFunc = allKnownNotifierFunc(cancel)
	case "current":
		cancelFunc = desiredStatusNotifierFunc(cancel, status.CurrentStatus)
	case "deleted":
		cancelFunc = desiredStatusNotifierFunc(cancel, status.NotFoundStatus)
	case "forever":
		cancelFunc = func(*collector.ResourceStatusCollector, event.Event) {}
	default:
		return fmt.Errorf("unknown value for pollUntil: %q", r.pollUntil)
	}

	eventChannel := statusPoller.Poll(ctx, printData.Identifiers, polling.PollOptions{
		PollInterval: r.period,
	})

	return printer.Print(eventChannel, printData.Identifiers, cancelFunc)
}

// desiredStatusNotifierFunc returns an Observer function for the
// ResourceStatusCollector that will cancel the context (using the cancelFunc)
// when all resources have reached the desired status.
func desiredStatusNotifierFunc(cancelFunc context.CancelFunc,
	desired status.Status) collector.ObserverFunc {
	return func(rsc *collector.ResourceStatusCollector, _ event.Event) {
		var rss []*event.ResourceStatus
		for _, rs := range rsc.ResourceStatuses {
			rss = append(rss, rs)
		}
		aggStatus := aggregator.AggregateStatus(rss, desired)
		if aggStatus == desired {
			cancelFunc()
		}
	}
}

// allKnownNotifierFunc returns an Observer function for the
// ResourceStatusCollector that will cancel the context (using the cancelFunc)
// when all resources have a known status.
func allKnownNotifierFunc(cancelFunc context.CancelFunc) collector.ObserverFunc {
	return func(rsc *collector.ResourceStatusCollector, _ event.Event) {
		for _, rs := range rsc.ResourceStatuses {
			if rs.Status == status.UnknownStatus {
				return
			}
		}
		cancelFunc()
	}
}

func pollerFactoryFunc(f cmdutil.Factory) (poller.Poller, error) {
	return polling.NewStatusPollerFromFactory(f, polling.Options{})
}

type Loader interface {
	GetInvInfo(cmd *cobra.Command, args []string) (inventory.Info, error)
}

type InventoryLoader struct {
	Loader manifestreader.ManifestLoader
}

func NewInventoryLoader(loader manifestreader.ManifestLoader) *InventoryLoader {
	return &InventoryLoader{
		Loader: loader,
	}
}

func (ir *InventoryLoader) GetInvInfo(cmd *cobra.Command, args []string) (inventory.Info, error) {
	_, err := common.DemandOneDirectory(args)
	if err != nil {
		return nil, err
	}

	reader, err := ir.Loader.ManifestReader(cmd.InOrStdin(), flagutils.PathFromArgs(args))
	if err != nil {
		return nil, err
	}
	objs, err := reader.Read()
	if err != nil {
		return nil, err
	}

	invObj, _, err := inventory.SplitUnstructureds(objs)
	if err != nil {
		return nil, err
	}
	inv := inventory.WrapInventoryInfoObj(invObj)
	return inv, nil
}
