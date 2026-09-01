package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stackrox/harness-openshell/internal/openshell"
	"golang.org/x/term"
)

func runInteractive(ctx context.Context, client openshell.SandboxExecutionClient, sandbox string, command []string, stdin io.Reader, stdout io.Writer) (exitCode int, runErr error) {
	cols, rows := uint32(80), uint32(24)
	resize := (<-chan os.Signal)(nil)
	var readSize func() (uint32, uint32, error)

	inputFile, inputIsFile := stdin.(*os.File)
	if inputIsFile && term.IsTerminal(int(inputFile.Fd())) {
		state, err := term.MakeRaw(int(inputFile.Fd()))
		if err != nil {
			return -1, fmt.Errorf("enabling terminal raw mode: %w", err)
		}
		defer func() {
			if err := term.Restore(int(inputFile.Fd()), state); err != nil {
				runErr = errors.Join(runErr, fmt.Errorf("restoring terminal mode: %w", err))
			}
		}()
	}

	sizeFile := inputFile
	if outputFile, ok := stdout.(*os.File); ok && term.IsTerminal(int(outputFile.Fd())) {
		sizeFile = outputFile
	}
	if sizeFile != nil && term.IsTerminal(int(sizeFile.Fd())) {
		readSize = func() (uint32, uint32, error) {
			width, height, err := term.GetSize(int(sizeFile.Fd()))
			return uint32(width), uint32(height), err
		}
		var err error
		cols, rows, err = readSize()
		if err != nil {
			return -1, fmt.Errorf("reading terminal size: %w", err)
		}
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGWINCH)
		defer signal.Stop(signals)
		resize = signals
	}

	session, err := client.ExecInteractive(ctx, sandbox, command, cols, rows)
	if err != nil {
		return -1, err
	}
	defer func() {
		if err := session.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("closing interactive session: %w", err))
		}
	}()
	if err := pumpInteractiveSession(ctx, session, stdin, stdout, resize, readSize); err != nil {
		return -1, err
	}
	exitCode, err = session.ExitCode()
	if err != nil {
		return -1, fmt.Errorf("reading interactive exit code: %w", err)
	}
	return exitCode, nil
}

func pumpInteractiveSession(ctx context.Context, session openshell.InteractiveSession, stdin io.Reader, stdout io.Writer, resize <-chan os.Signal, readSize func() (uint32, uint32, error)) error {
	outputDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(stdout, session)
		outputDone <- err
	}()
	inputDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(session, stdin)
		inputDone <- err
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-outputDone:
			if err != nil {
				return fmt.Errorf("reading interactive output: %w", err)
			}
			return nil
		case err := <-inputDone:
			if err != nil {
				return fmt.Errorf("writing interactive input: %w", err)
			}
			inputDone = nil
		case _, ok := <-resize:
			if !ok {
				resize = nil
				continue
			}
			cols, rows, err := readSize()
			if err != nil {
				return fmt.Errorf("reading terminal size: %w", err)
			}
			if err := session.Resize(cols, rows); err != nil {
				return fmt.Errorf("resizing interactive session: %w", err)
			}
		}
	}
}
