package gitpanel

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

const writeQueueBufferSize = 64

var writeRetryBackoffs = []time.Duration{
	80 * time.Millisecond,
	160 * time.Millisecond,
	320 * time.Millisecond,
}

type gitRunner func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error)
type backoffSleeper func(ctx context.Context, d time.Duration) error

type queuedWriteCommand struct {
	requestCtx context.Context
	commandID  string
	action     string
	timeout    time.Duration
	run        func(context.Context) error
	result     chan error
	diag       *commandDiagnosticState
}

type repoCommandQueue struct {
	repoRoot string
	items    chan queuedWriteCommand
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) executeWrite(repoRoot string, commandID string, action string, args []string, startedAt time.Time, timeout time.Duration, run func(context.Context, *commandDiagnosticState) error) error {
	if timeout <= 0 {
		timeout = defaultWriteTimeout
	}

	diag := newCommandDiagnosticState(commandID, repoRoot, action, args, startedAt)
	if run == nil {
		err := NewBindingError(
			CodeUnknown,
			"Comando write inválido.",
			"A execução interna não foi fornecida.",
		)
		s.emitCommandDiagnostic(diag, commandStatusFailed, err)
		return err
	}

	command := queuedWriteCommand{
		requestCtx: context.Background(),
		commandID:  commandID,
		action:     action,
		timeout:    timeout,
		run: func(ctx context.Context) error {
			return run(ctx, diag)
		},
		result: make(chan error, 1),
		diag:   diag,
	}

	if err := s.enqueueWrite(repoRoot, command); err != nil {
		s.emitCommandDiagnostic(diag, commandStatusFailed, err)
		return err
	}

	s.emitCommandDiagnostic(diag, commandStatusSucceeded, nil)
	return nil
}

func (s *Service) enqueueWrite(repoRoot string, command queuedWriteCommand) error {
	queue, err := s.getOrCreateRepoQueue(repoRoot)
	if err != nil {
		return err
	}

	if command.requestCtx == nil {
		command.requestCtx = context.Background()
	}
	if command.result == nil {
		command.result = make(chan error, 1)
	}

	select {
	case queue.items <- command:
		s.emitCommandDiagnostic(command.diag, commandStatusQueued, nil)
	case <-command.requestCtx.Done():
		if mapped := queueErrorFromContext(command.requestCtx.Err(), "Comando cancelado antes de entrar na fila."); mapped != nil {
			return mapped
		}
		return command.requestCtx.Err()
	case <-s.shutdownCtx.Done():
		return serviceClosedError()
	}

	select {
	case err := <-command.result:
		return err
	case <-command.requestCtx.Done():
		if mapped := queueErrorFromContext(command.requestCtx.Err(), "Comando cancelado enquanto aguardava execução."); mapped != nil {
			return mapped
		}
		return command.requestCtx.Err()
	case <-s.shutdownCtx.Done():
		return serviceClosedError()
	}
}

func (s *Service) getOrCreateRepoQueue(repoRoot string) (*repoCommandQueue, error) {
	normalized := filepath.Clean(strings.TrimSpace(repoRoot))
	if normalized == "" || normalized == "." {
		return nil, NewBindingError(
			CodeRepoNotResolved,
			"Repositório não resolvido para comando write.",
			"Informe um caminho de repositório válido.",
		)
	}
	if s.closed.Load() {
		return nil, serviceClosedError()
	}

	s.queueMu.Lock()
	defer s.queueMu.Unlock()

	if s.closed.Load() {
		return nil, serviceClosedError()
	}

	if queue, ok := s.queues[normalized]; ok {
		return queue, nil
	}

	queue := &repoCommandQueue{
		repoRoot: normalized,
		items:    make(chan queuedWriteCommand, writeQueueBufferSize),
	}
	s.queues[normalized] = queue

	s.workerWG.Add(1)
	go s.runRepoQueueWorker(queue)
	return queue, nil
}

func (s *Service) runRepoQueueWorker(queue *repoCommandQueue) {
	defer s.workerWG.Done()

	for {
		select {
		case <-s.shutdownCtx.Done():
			return
		case command := <-queue.items:
			if command.run == nil {
				s.emitCommandDiagnostic(command.diag, commandStatusFailed, NewBindingError(
					CodeUnknown,
					"Comando write inválido.",
					"A execução interna não foi fornecida.",
				))
				select {
				case command.result <- NewBindingError(
					CodeUnknown,
					"Comando write inválido.",
					"A execução interna não foi fornecida.",
				):
				default:
				}
				continue
			}

			s.emitCommandDiagnostic(command.diag, commandStatusStarted, nil)
			commandCtx, cancel := buildQueueCommandContext(s.shutdownCtx, command.requestCtx, command.timeout)
			runErr := command.run(commandCtx)
			if runErr == nil {
				if mapped := queueErrorFromContext(commandCtx.Err(), "Comando interrompido por cancelamento."); mapped != nil {
					runErr = mapped
				}
			}
			cancel()

			select {
			case command.result <- runErr:
			default:
			}
		}
	}
}

func buildQueueCommandContext(base context.Context, requestCtx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if base == nil {
		base = context.Background()
	}

	ctx := base
	cancelers := make([]func(), 0, 2)

	if requestCtx != nil {
		withRequestCancel, requestCancel := context.WithCancel(ctx)
		stop := context.AfterFunc(requestCtx, requestCancel)
		cancelers = append(cancelers, func() {
			stop()
			requestCancel()
		})
		ctx = withRequestCancel
	}

	if timeout > 0 {
		withTimeout, timeoutCancel := context.WithTimeout(ctx, timeout)
		cancelers = append(cancelers, timeoutCancel)
		ctx = withTimeout
	}

	if len(cancelers) == 0 {
		return ctx, func() {}
	}

	return ctx, func() {
		for i := len(cancelers) - 1; i >= 0; i-- {
			cancelers[i]()
		}
	}
}

func (s *Service) runWriteGitWithRetry(ctx context.Context, diag *commandDiagnosticState, stdin string, args ...string) (string, string, int, error) {
	var (
		stdout   string
		stderr   string
		exitCode int
		runErr   error
	)

	for attempt := 0; ; attempt++ {
		attemptTimeout := remainingTimeout(ctx, defaultWriteTimeout)
		stdout, stderr, exitCode, runErr = s.runGit(ctx, attemptTimeout, stdin, args...)
		if diag != nil {
			diag.recordAttempt(args, stderr, exitCode, attempt+1)
		}
		if runErr == nil {
			return stdout, stderr, exitCode, nil
		}

		if mapped := queueErrorFromContext(runErr, "Comando Git interrompido."); mapped != nil {
			return stdout, stderr, exitCode, mapped
		}

		if !isTransientIndexLockError(stderr, runErr) || attempt >= len(writeRetryBackoffs) {
			return stdout, stderr, exitCode, runErr
		}
		s.emitCommandDiagnostic(diag, commandStatusRetried, nil)

		if sleepErr := s.sleep(ctx, writeRetryBackoffs[attempt]); sleepErr != nil {
			if mapped := queueErrorFromContext(sleepErr, "Retry de index.lock interrompido."); mapped != nil {
				return stdout, stderr, exitCode, mapped
			}
			return stdout, stderr, exitCode, sleepErr
		}
	}
}

func remainingTimeout(ctx context.Context, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = defaultWriteTimeout
	}
	if ctx == nil {
		return fallback
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		return fallback
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Millisecond
	}
	if remaining < fallback {
		return remaining
	}
	return fallback
}

func queueErrorFromContext(err error, details string) error {
	if err == nil {
		return nil
	}

	if bindingErr := AsBindingError(err); bindingErr != nil {
		if bindingErr.Code == CodeTimeout || bindingErr.Code == CodeCanceled {
			return bindingErr
		}
		return nil
	}

	trimmedDetails := strings.TrimSpace(details)
	if errors.Is(err, context.DeadlineExceeded) {
		return NewBindingError(
			CodeTimeout,
			"Comando Git excedeu o tempo limite.",
			trimmedDetails,
		)
	}
	if errors.Is(err, context.Canceled) {
		return NewBindingError(
			CodeCanceled,
			"Comando Git cancelado.",
			trimmedDetails,
		)
	}
	return nil
}

func isTransientIndexLockError(stderr string, runErr error) bool {
	combined := strings.ToLower(strings.TrimSpace(stderr))
	if runErr != nil {
		if combined != "" {
			combined += " | "
		}
		combined += strings.ToLower(runErr.Error())
	}

	if !strings.Contains(combined, "index.lock") {
		return false
	}

	if strings.Contains(combined, "another git process") ||
		strings.Contains(combined, "file exists") ||
		strings.Contains(combined, "unable to create") ||
		strings.Contains(combined, "could not lock") {
		return true
	}

	return true
}

func serviceClosedError() error {
	return NewBindingError(
		CodeServiceUnavailable,
		"Serviço Git Panel em encerramento.",
		"A fila de comandos write foi cancelada durante o shutdown.",
	)
}

func (s *Service) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	if s.shutdownCancel != nil {
		s.shutdownCancel()
	}

	workersDone := make(chan struct{})
	go func() {
		s.workerWG.Wait()
		close(workersDone)
	}()

	select {
	case <-workersDone:
		return nil
	case <-ctx.Done():
		if mapped := queueErrorFromContext(ctx.Err(), "Timeout aguardando encerramento da fila de comandos."); mapped != nil {
			return mapped
		}
		return ctx.Err()
	}
}
