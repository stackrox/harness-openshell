package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type Orchestrator struct {
	config   *OrchestratorConfig
	adapter  HarnessAdapter
	session  *SessionWriter
	configDir string
	cycle    int
	failures int
}

func New(cfg *OrchestratorConfig, configDir string) (*Orchestrator, error) {
	adapter, err := NewAdapter(cfg.Entrypoint)
	if err != nil {
		return nil, err
	}
	session, err := NewSessionWriter(cfg.SessionDir)
	if err != nil {
		return nil, err
	}
	return &Orchestrator{
		config:    cfg,
		adapter:   adapter,
		session:   session,
		configDir: configDir,
	}, nil
}

func (o *Orchestrator) Run(ctx context.Context) error {
	defer o.session.Close()

	switch o.config.Mode {
	case "once":
		result, err := o.runCycle(ctx)
		if err != nil {
			return err
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("agent exited with code %d", result.ExitCode)
		}
		return nil
	case "watch":
		return o.runWatch(ctx)
	default:
		return fmt.Errorf("unknown mode %q", o.config.Mode)
	}
}

func (o *Orchestrator) runWatch(ctx context.Context) error {
	for {
		result, err := o.runCycle(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			o.failures++
			logf("cycle %d failed: %v (failure %d/%d)", o.cycle, err, o.failures, o.config.MaxFailures)
			if o.failures >= o.config.MaxFailures {
				return fmt.Errorf("max transient failures (%d) exceeded", o.config.MaxFailures)
			}
			o.sleepWithHeartbeat(ctx, o.config.PollInterval)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		o.runHooks(result)

		if result.IsTerminal() {
			logf("cycle %d: terminal status %q (%s)", o.cycle, result.Status, result.Reason)
			return nil
		}

		switch result.Status {
		case "transient_failure":
			o.failures++
			logf("cycle %d: transient failure (%s), failure %d/%d", o.cycle, result.Reason, o.failures, o.config.MaxFailures)
			if o.failures >= o.config.MaxFailures {
				return fmt.Errorf("max transient failures (%d) exceeded", o.config.MaxFailures)
			}
		default:
			o.failures = 0
		}

		poll := o.config.PollInterval
		if result.PollSeconds > 0 {
			poll = result.PollSeconds
		}

		logf("cycle %d: %s (%s), next cycle in %ds", o.cycle, result.Status, result.Reason, poll)
		o.sleepWithHeartbeat(ctx, poll)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (o *Orchestrator) runCycle(ctx context.Context) (*CycleResult, error) {
	o.cycle++
	start := time.Now()

	task := o.resolveTask()
	headless := !o.config.TTY
	cmd := o.adapter.BuildCommand(task, headless)
	cmd.Stderr = os.Stderr

	var stdout bytes.Buffer
	if headless {
		cmd.Stdout = &stdout
	} else {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
	}

	logf("cycle %d: running %s", o.cycle, o.adapter.Name())
	err := cmd.Run()
	duration := time.Since(start)

	var result *CycleResult
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("running %s: %w", o.adapter.Name(), err)
		}
	}

	if o.config.Sentinel && headless {
		parsed, parseErr := ParseSentinel(stdout.Bytes())
		if parseErr != nil {
			logf("cycle %d: sentinel parse error: %v", o.cycle, parseErr)
			result = &CycleResult{Status: "transient_failure", Reason: "malformed_sentinel", ExitCode: exitCode}
		} else if parsed != nil {
			result = parsed
			result.ExitCode = exitCode
		}
	}

	if result == nil {
		result = SyntheticResult(exitCode)
	}

	record := SessionRecord{
		Timestamp:   start,
		Cycle:       o.cycle,
		Entrypoint:  o.adapter.Name(),
		Mode:        o.config.Mode,
		DurationSec: duration.Seconds(),
		Result:      result,
	}
	if writeErr := o.session.Write(record); writeErr != nil {
		logf("warning: failed to write session record: %v", writeErr)
	}

	return result, nil
}

func (o *Orchestrator) resolveTask() string {
	if o.config.Task == "" {
		return ""
	}
	data, err := os.ReadFile(o.config.Task)
	if err != nil {
		logf("warning: could not read task file %s: %v", o.config.Task, err)
		return ""
	}
	return string(data)
}

func (o *Orchestrator) sleepWithHeartbeat(ctx context.Context, seconds int) {
	if o.config.Heartbeat <= 0 {
		select {
		case <-time.After(time.Duration(seconds) * time.Second):
		case <-ctx.Done():
		}
		return
	}

	deadline := time.After(time.Duration(seconds) * time.Second)
	tick := time.NewTicker(time.Duration(o.config.Heartbeat) * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			return
		case <-tick.C:
			logf("heartbeat: cycle %d, sleeping", o.cycle)
		case <-ctx.Done():
			return
		}
	}
}

func (o *Orchestrator) runHooks(result *CycleResult) {
	var hooks []string
	switch result.Status {
	case "complete":
		hooks = o.config.OnComplete
	case "propose":
		hooks = o.config.OnPropose
	}
	for _, h := range hooks {
		cmd := exec.Command("sh", "-c", h)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			logf("hook failed: %v", err)
		}
	}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "harness-orchestrator: "+format+"\n", args...)
}
