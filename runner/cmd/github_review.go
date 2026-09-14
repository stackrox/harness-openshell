package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/integrations/github/review"
	"github.com/stackrox/harness-openshell/runner/internal/openshell"
)

// NewGitHubCmd keeps GitHub-specific inputs out of the generic workflow commands.
func NewGitHubCmd(newClient openshell.Factory) *cobra.Command {
	github := &cobra.Command{Use: "github", Short: "Run GitHub repository integrations"}
	reviews := &cobra.Command{Use: "review", Short: "Prepare and run a sandboxed pull-request review"}
	github.AddCommand(reviews)
	for _, phase := range []string{"prepare", "run"} {
		var o review.Options
		var gatewayName, workspace *string
		command := &cobra.Command{
			Use: phase, Short: phase + " a pull-request review", Args: cobra.NoArgs,
			RunE: func(command *cobra.Command, _ []string) error {
				if phase == "run" {
					o.Gateway, o.Workspace = *gatewayName, *workspace
				}
				s := review.Service{Execute: reviewExecutor(newClient)}
				if phase == "prepare" {
					_, err := s.Prepare(command.Context(), o)
					return err
				}
				return s.Run(command.Context(), o)
			},
		}
		flags := command.Flags()
		flags.StringVar(&o.Dir, "dir", os.Getenv("REVIEW_DIR"), "Absolute directory for prepared input and review artifacts")
		flags.StringVar(&o.Repository, "repo", os.Getenv("REVIEW_REPOSITORY"), "GitHub owner/repository")
		pr, _ := strconv.Atoi(os.Getenv("REVIEW_PR"))
		flags.IntVar(&o.PR, "pr", pr, "Pull request number")
		flags.StringVar(&o.Head, "head", os.Getenv("REVIEW_HEAD"), "Expected head commit SHA")
		flags.BoolVar(&o.AllowDrafts, "allow-drafts", os.Getenv("ALLOW_DRAFT_REVIEWS") == "true", "Allow explicitly labeled draft PRs")
		if phase == "run" {
			flags.StringVarP(&o.Workflow, "file", "f", "tasks/github-pr-reviewer/workflow/opencode-harness.yaml", "Trusted workflow with existing target and provider references")
			flags.StringVar(&o.Skill, "skill", "tasks/github-pr-reviewer/workflow/skills/pr-review/SKILL.md", "Trusted review skill file")
			flags.StringVar(&o.SkillRoot, "skill-root", "", "Constrain an explicit relative skill path to this trusted checkout")
			flags.StringVar(&o.PolicyTemplate, "policy-template", "tasks/github-pr-reviewer/openshell/policy.yaml", "Trusted native OpenShell policy template")
			flags.StringVar(&o.GitHubProvider, "github-provider", "github-review", "Existing endpointless GitHub provider bound by the policy")
			gatewayName, workspace = registerTargetFlags(command)
		}
		reviews.AddCommand(command)
	}
	var outputDir string
	validate := &cobra.Command{Use: "validate-output", Short: "Validate an OpenCode review event stream", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := review.ValidateOutput(filepath.Join(outputDir, "agent.ndjson"))
			return err
		},
	}
	validate.Flags().StringVar(&outputDir, "dir", "", "Review artifact directory")
	_ = validate.MarkFlagRequired("dir")
	reviews.AddCommand(validate)
	return github
}

// The adapter supplies review data and streams; the existing apply service
// remains the sole owner of SDK planning, execution, results, and cleanup.
func reviewExecutor(newClient openshell.Factory) review.Executor {
	return func(ctx context.Context, task review.Task, stdout, stderr io.Writer) error {
		return runApply(ctx, newClient, applyRequest{
			File: task.File, Name: task.Name, Gateway: task.Gateway, Workspace: task.Workspace,
			ResultFile: task.ResultFile, OutputDir: task.OutputDir, Variables: task.Variables,
			Stdout: stdout, Stderr: stderr, RequireExistingInference: true, RequireEphemeral: true,
		}, stderr)
	}
}
