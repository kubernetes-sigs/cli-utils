package diff

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/diff"
	"k8s.io/kubectl/pkg/cmd/util"
)

func NewCmdDiff(f util.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	return diff.NewCmdDiff(f, ioStreams)
}
