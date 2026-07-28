package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/spf13/cobra"
)

func TestEveryProductLeafHasDescriptionAndExample(t *testing.T) {
	t.Parallel()

	root := New(buildinfo.Info{})
	for _, child := range root.Commands() {
		if child.Name() == "completion" || child.Name() == "help" {
			continue
		}
		assertLeafHelp(t, child)
	}
}

func TestRootHelpPresentsPrimaryServicesAndGlobalBehavior(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	command := New(buildinfo.Info{})
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"store", "queue", "workflow", "server", "cluster", "acl", "pubsub",
		"--output", "--timeout",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("root help does not contain %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "--profile") {
		t.Fatalf("root help exposes advanced profile override:\n%s", output.String())
	}
	for _, nested := range []string{"schedule", "governance"} {
		if rootHasCommand(command, nested) {
			t.Fatalf("root exposes workflow-owned command %q", nested)
		}
		if !workflowHasCommand(command, nested) {
			t.Fatalf("workflow does not expose nested command %q", nested)
		}
	}
}

func rootHasCommand(root *cobra.Command, name string) bool {
	for _, command := range root.Commands() {
		if command.Name() == name {
			return true
		}
	}
	return false
}

func workflowHasCommand(root *cobra.Command, name string) bool {
	for _, command := range root.Commands() {
		if command.Name() == "workflow" {
			return rootHasCommand(command, name)
		}
	}
	return false
}

func TestCompletionRecommendsNextQueueCommand(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	command := New(buildinfo.Info{})
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"__complete", "queue", ""})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"enqueue", "claim", "complete", "describe"} {
		if !completionContains(output.String(), want) {
			t.Errorf("queue completion does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestCompletionNestsScheduleAndGovernanceUnderWorkflow(t *testing.T) {
	t.Parallel()

	var workflowOutput bytes.Buffer
	workflow := New(buildinfo.Info{})
	workflow.SetOut(&workflowOutput)
	workflow.SetErr(&workflowOutput)
	workflow.SetArgs([]string{"__complete", "workflow", ""})
	if err := workflow.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"schedule", "governance"} {
		if !completionContains(workflowOutput.String(), want) {
			t.Errorf("workflow completion does not contain %q:\n%s", want, workflowOutput.String())
		}
	}

	var rootOutput bytes.Buffer
	root := New(buildinfo.Info{})
	root.SetOut(&rootOutput)
	root.SetErr(&rootOutput)
	root.SetArgs([]string{"__complete", ""})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, nested := range []string{"schedule", "governance"} {
		if completionContains(rootOutput.String(), nested) {
			t.Errorf("root completion exposes nested command %q:\n%s", nested, rootOutput.String())
		}
	}
}

func TestCompletionRecommendsFlowStates(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	command := New(buildinfo.Info{})
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"__complete", "queue", "claim", "email", "--state", ""})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"queued", "running", "failed"} {
		if !completionContains(output.String(), want) {
			t.Errorf("state completion does not contain %q:\n%s", want, output.String())
		}
	}
}

func completionContains(output, candidate string) bool {
	for _, line := range strings.Split(output, "\n") {
		if line == candidate || strings.HasPrefix(line, candidate+"\t") {
			return true
		}
	}
	return false
}
