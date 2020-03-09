// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package printers

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/integer"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/collector"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

const (
	// updateInterval defines how often the printer will update the UI.
	updateInterval = 1 * time.Second
)

// printContentFunc defines the function type used by the printer to ask a
// ColumnDef to output the appropriate content for a given resource and column.
type printContentFunc func(w io.Writer, width int,
	resource *event.ResourceStatus) (int, error)

// ColumnDef defines the properties of every supported column by the printer.
type ColumnDef struct {
	// name of the column. This is just a string that is unique among all the
	// defined columns
	name string
	// header defines the title of the column that will be printed in the table.
	header string
	// width defines the width of the column
	width int
	// printContent defines a function that will be called by the printer
	// to output information into the given cell in the table for a provided
	// ResourceStatus. This function is responsible for trimming any content
	// that is too wide and takes care of setting the color of anything that
	// is printed.
	printContent printContentFunc
}

var (
	// columns contains a list of ColumnDefs, one for each of the columns that
	// is supported by the printer.
	columns = []ColumnDef{
		{
			// Column containing the namespace of the resource.
			name:   "namespace",
			header: "NAMESPACE",
			width:  10,
			printContent: func(w io.Writer, width int, r *event.ResourceStatus) (int,
				error) {
				namespace := r.Identifier.Namespace
				if len(namespace) > width {
					namespace = namespace[:width]
				}
				_, err := fmt.Fprint(w, namespace)
				return len(namespace), err
			},
		},
		{
			// Column containing the resource type and name. Currently it does not
			// print group or version since those are rarely needed to uniquely
			// distinguish two resources from each other. Just name and kind should
			// be enough in almost all cases and saves space in the output.
			name:   "resource",
			header: "RESOURCE",
			width:  40,
			printContent: func(w io.Writer, width int, r *event.ResourceStatus) (int,
				error) {
				text := fmt.Sprintf("%s/%s", r.Identifier.GroupKind.Kind,
					r.Identifier.Name)
				if len(text) > width {
					text = text[:width]
				}
				_, err := fmt.Fprint(w, text)
				return len(text), err
			},
		},
		{
			// Column containing the status of the resource as computed by the
			// status and polling libraries.
			name:   "status",
			header: "STATUS",
			width:  10,
			printContent: func(w io.Writer, width int, r *event.ResourceStatus) (int,
				error) {
				s := r.Status.String()
				if len(s) > width {
					s = s[:width]
				}
				color, setColor := colorForTableStatus(r.Status)
				var outputStatus string
				if setColor {
					outputStatus = sPrintWithColor(color, s)
				} else {
					outputStatus = s
				}
				_, err := fmt.Fprint(w, outputStatus)
				return len(s), err
			},
		},
		{
			// Column containing the conditions available on the resource. It uses
			// colors to show the status of each of the conditions.
			name:   "conditions",
			header: "CONDITIONS",
			width:  40,
			printContent: func(w io.Writer, width int, r *event.ResourceStatus) (int,
				error) {
				u := r.Resource
				if u == nil {
					return fmt.Fprintf(w, "-")
				}

				conditions, found, err := unstructured.NestedSlice(u.Object,
					"status", "conditions")
				if !found || err != nil || len(conditions) == 0 {
					return fmt.Fprintf(w, "<None>")
				}

				realLength := 0
				for i, cond := range conditions {
					condition := cond.(map[string]interface{})
					conditionType := condition["type"].(string)
					conditionStatus := condition["status"].(string)
					var color color
					switch conditionStatus {
					case "True":
						color = GREEN
					case "False":
						color = RED
					default:
						color = YELLOW
					}
					remainingWidth := width - realLength
					if len(conditionType) > remainingWidth {
						conditionType = conditionType[:remainingWidth]
					}
					_, err := fmt.Fprint(w, sPrintWithColor(color, conditionType))
					if err != nil {
						return realLength, err
					}
					realLength += len(conditionType)
					if i < len(conditions)-1 && width-realLength > 2 {
						_, err = fmt.Fprintf(w, ",")
						if err != nil {
							return realLength, err
						}
						realLength += 1
					}
				}
				return realLength, nil
			},
		},
		{
			// Column shows the age of the resource. This is computed based on the
			// value of the creationTimestamp field.
			name:   "age",
			header: "AGE",
			width:  6,
			printContent: func(w io.Writer, width int, r *event.ResourceStatus) (int,
				error) {
				u := r.Resource
				if u == nil {
					return fmt.Fprint(w, "-")
				}

				timestamp, found, err := unstructured.NestedString(u.Object,
					"metadata", "creationTimestamp")
				if !found || err != nil || timestamp == "" {
					return fmt.Fprint(w, "-")
				}
				parsedTime, err := time.Parse(time.RFC3339, timestamp)
				if err != nil {
					return fmt.Fprint(w, "-")
				}
				age := time.Since(parsedTime)
				switch {
				case age.Seconds() <= 90:
					return fmt.Fprintf(w, "%ds",
						integer.RoundToInt32(age.Round(time.Second).Seconds()))
				case age.Minutes() <= 90:
					return fmt.Fprintf(w, "%dm",
						integer.RoundToInt32(age.Round(time.Minute).Minutes()))
				default:
					return fmt.Fprintf(w, "%dh",
						integer.RoundToInt32(age.Round(time.Hour).Hours()))
				}
			},
		},
		{
			// Column shows more detailed information regarding the status of the
			// resource. If there was an error encountered while fetching the
			// resource or computing status, the error message is shown in this
			// column.
			name:   "message",
			header: "MESSAGE",
			width:  40,
			printContent: func(w io.Writer, width int, r *event.ResourceStatus) (int,
				error) {
				var message string
				if r.Error != nil {
					message = r.Error.Error()
				} else {
					message = r.Message
				}
				if len(message) > width {
					message = message[:width]
				}
				return fmt.Fprint(w, message)
			},
		},
	}
)

// tablePrinter is an implementation of the Printer interface that outputs
// status information about resources in a table format with in-place updates.
type tablePrinter struct {
	collector *collector.ResourceStatusCollector
	w         io.Writer
}

// NewTablePrinter returns a new instance of the tablePrinter. The passed in
// collector is the source of data to be printed, and the writer is where the
// printer will send the output.
func NewTablePrinter(collector *collector.ResourceStatusCollector,
	w io.Writer) *tablePrinter {
	return &tablePrinter{
		collector: collector,
		w:         w,
	}
}

// Print prints the table of resources with their statuses until the
// provided stop channel is closed.
func (t *tablePrinter) Print(stop <-chan struct{}) <-chan struct{} {
	completed := make(chan struct{})

	linesPrinted := t.printTable(t.collector.LatestObservation(), 0)

	go func() {
		defer close(completed)
		ticker := time.NewTicker(updateInterval)
		for {
			select {
			case <-stop:
				ticker.Stop()
				latestObservation := t.collector.LatestObservation()
				if latestObservation.Error != nil {
					t.printError(latestObservation)
					return
				}
				linesPrinted = t.printTable(latestObservation, linesPrinted)
				return
			case <-ticker.C:
				latestObservation := t.collector.LatestObservation()
				linesPrinted = t.printTable(latestObservation, linesPrinted)
			}
		}
	}()

	return completed
}

func (t *tablePrinter) printError(data *collector.Observation) {
	t.printOrDie("Error: %s\n", data.Error.Error())
}

// printTable prints the table of resources with their status information.
// The provided moveUpCount value tells the function how many lines below the
// top of the table the cursor is currently at. The return value tells how
// many lines the function printed. This information is needed to make sure
// the function can reposition the cursor to the correct place each time a
// new version of the table is printed.
func (t *tablePrinter) printTable(data *collector.Observation,
	moveUpCount int) int {
	for i := 0; i < moveUpCount; i++ {
		t.moveUp()
		t.eraseCurrentLine()
	}
	linePrintCount := 0

	color, setColor := colorForTableStatus(data.AggregateStatus)
	var aggStatusText string
	if setColor {
		aggStatusText = sPrintWithColor(color, data.AggregateStatus.String())
	} else {
		aggStatusText = data.AggregateStatus.String()
	}
	t.printOrDie("Aggregate status: %s\n", aggStatusText)
	linePrintCount++

	for i, column := range columns {
		format := fmt.Sprintf("%%-%ds", column.width)
		t.printOrDie(format, column.header)
		if i == len(columns)-1 {
			t.printOrDie("\n")
			linePrintCount++
		} else {
			t.printOrDie("  ")
		}
	}

	for _, resource := range data.ResourceStatuses {
		for i, column := range columns {
			written, err := column.printContent(t.w, column.width, resource)
			if err != nil {
				panic(err)
			}
			remainingSpace := column.width - written
			t.printOrDie(strings.Repeat(" ", remainingSpace))
			if i == len(columns)-1 {
				t.printOrDie("\n")
				linePrintCount++
			} else {
				t.printOrDie("  ")
			}
		}

		linePrintCount += t.printSubTable(resource.GeneratedResources, "")
	}

	return linePrintCount
}

// printSubTable prints out any generated resources that belong to the
// top-level resources. This function takes care of printing the correct tree
// structure and indentation.
func (t *tablePrinter) printSubTable(resources []*event.ResourceStatus,
	prefix string) int {
	linePrintCount := 0
	for j, resource := range resources {
		for i, column := range columns {
			availableWidth := column.width
			if column.name == "resource" {
				if j < len(resources)-1 {
					t.printOrDie(prefix + `├─ `)
				} else {
					t.printOrDie(prefix + `└─ `)
				}
				availableWidth -= utf8.RuneCountInString(prefix) + 3
			}
			written, err := column.printContent(t.w, availableWidth, resource)
			if err != nil {
				panic(err)
			}
			remainingSpace := availableWidth - written
			t.printOrDie(strings.Repeat(" ", remainingSpace))
			if i == len(columns)-1 {
				t.printOrDie("\n")
				linePrintCount++
			} else {
				t.printOrDie("  ")
			}
		}

		var prefix string
		if j < len(resources)-1 {
			prefix = `│  `
		} else {
			prefix = "   "
		}
		linePrintCount += t.printSubTable(resource.GeneratedResources, prefix)
	}
	return linePrintCount
}

func (t *tablePrinter) printOrDie(format string, a ...interface{}) {
	_, err := fmt.Fprintf(t.w, format, a...)
	if err != nil {
		panic(err)
	}
}

func (t *tablePrinter) moveUp() {
	t.printOrDie("%c[%dA", ESC, 1)
}

func (t *tablePrinter) eraseCurrentLine() {
	t.printOrDie("%c[2K\r", ESC)
}

func sPrintWithColor(color color, format string, a ...interface{}) string {
	return fmt.Sprintf("%c[%dm", ESC, color) +
		fmt.Sprintf(format, a...) +
		fmt.Sprintf("%c[%dm", ESC, RESET)
}

func colorForTableStatus(s status.Status) (color color, setColor bool) {
	switch s {
	case status.CurrentStatus:
		color = GREEN
		setColor = true
	case status.InProgressStatus:
		color = YELLOW
		setColor = true
	case status.FailedStatus:
		color = RED
		setColor = true
	}
	return
}
