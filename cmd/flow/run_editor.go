package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/samleeney/flows/pkg/editor"
	"github.com/samleeney/flows/pkg/live"
)

const editorStartTimeout = 5 * time.Second

type runEditorSession struct {
	BaseURL     string
	Descriptors []live.Descriptor
	close       func()
}

func (s runEditorSession) Close() {
	if s.close != nil {
		s.close()
	}
}

func ensureRunEditor(filePath, canonical, flowKey string, persistent bool) (runEditorSession, error) {
	if descs, _ := discoverCurrentEditorDescriptors(flowKey); len(descs) > 0 {
		descs = sortDescriptors(descs)
		return runEditorSession{BaseURL: descs[0].BaseURL, Descriptors: descs}, nil
	}

	if persistent {
		return startPersistentRunEditor(filePath, flowKey)
	}
	return startInlineRunEditor(filePath, canonical, flowKey)
}

func startInlineRunEditor(filePath, canonical, flowKey string) (runEditorSession, error) {
	token, err := live.NewToken()
	if err != nil {
		return runEditorSession{}, fmt.Errorf("generate live token: %w", err)
	}

	srv, err := editor.NewServer(editor.NewServerOptions{
		FilePath:      filePath,
		CanonicalPath: canonical,
		FlowKey:       flowKey,
		Token:         token,
		UIFS:          embeddedUI(),
	})
	if err != nil {
		return runEditorSession{}, fmt.Errorf("creating editor: %w", err)
	}

	if err := srv.StartFileWatcher(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: editor file watcher failed: %v\n", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		srv.Close()
		return runEditorSession{}, fmt.Errorf("bind editor: %w", err)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	cleanupDescriptor, _, err := live.RegisterDescriptor(live.Descriptor{
		BaseURL:       baseURL,
		Token:         token,
		FlowKey:       flowKey,
		CanonicalPath: canonical,
	})
	if err != nil {
		srv.Close()
		return runEditorSession{}, fmt.Errorf("register live descriptor: %w", err)
	}

	if descs, err := waitForEditorDescriptor(flowKey, 0, editorStartTimeout); err == nil && len(descs) > 0 {
		return runEditorSession{
			BaseURL:     baseURL,
			Descriptors: descs,
			close: func() {
				cleanupDescriptor()
				srv.Close()
				select {
				case <-errCh:
				case <-time.After(200 * time.Millisecond):
				}
			},
		}, nil
	}

	cleanupDescriptor()
	srv.Close()
	select {
	case err := <-errCh:
		if err != nil {
			return runEditorSession{}, fmt.Errorf("starting editor server: %w", err)
		}
	default:
	}
	return runEditorSession{}, fmt.Errorf("starting editor server: timed out")
}

func startPersistentRunEditor(filePath, flowKey string) (runEditorSession, error) {
	exe, err := os.Executable()
	if err != nil {
		return runEditorSession{}, fmt.Errorf("locating flow executable: %w", err)
	}

	logFile, logPath, err := openFlowLog("chart", flowKey)
	if err != nil {
		return runEditorSession{}, err
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "chart", filePath, "--no-open", "--port", "0")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}

	if err := cmd.Start(); err != nil {
		return runEditorSession{}, fmt.Errorf("starting editor: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()

	time.Sleep(100 * time.Millisecond)
	descs, err := waitForEditorDescriptor(flowKey, pid, editorStartTimeout)
	if err != nil {
		return runEditorSession{}, fmt.Errorf("%w; chart log: %s", err, logPath)
	}
	descs = sortDescriptors(descs)
	return runEditorSession{BaseURL: descs[0].BaseURL, Descriptors: descs}, nil
}

func waitForEditorDescriptor(flowKey string, pid int, timeout time.Duration) ([]live.Descriptor, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		descs, err := discoverCurrentEditorDescriptors(flowKey)
		if err == nil && len(descs) > 0 {
			return sortDescriptors(descs), nil
		}
		if err != nil {
			lastErr = err
		}
		if pid > 0 && !live.ProcessExists(pid) {
			return nil, fmt.Errorf("editor process exited before registering")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("waiting for editor: %w", lastErr)
	}
	return nil, fmt.Errorf("waiting for editor: timed out")
}

func discoverCurrentEditorDescriptors(flowKey string) ([]live.Descriptor, error) {
	descs, err := live.DiscoverDescriptors(flowKey)
	if err != nil || len(descs) == 0 {
		return descs, err
	}

	exe, err := os.Executable()
	if err != nil {
		return descs, nil
	}
	info, err := os.Stat(exe)
	if err != nil {
		return descs, nil
	}
	builtAt := info.ModTime().UnixNano()
	current := descs[:0]
	for _, d := range descs {
		if d.CreatedAtUnixNano >= builtAt {
			current = append(current, d)
		}
	}
	return current, nil
}

func startBackgroundForegroundRun(flowKey string) (int, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, "", fmt.Errorf("locating flow executable: %w", err)
	}

	logFile, logPath, err := openFlowLog("run", flowKey)
	if err != nil {
		return 0, "", err
	}
	defer logFile.Close()

	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return 0, "", fmt.Errorf("opening %s: %w", os.DevNull, err)
	}
	defer stdin.Close()

	args := append([]string{}, os.Args[1:]...)
	args = append(args, "--foreground")
	cmd := exec.Command(exe, args...)
	cmd.Stdin = stdin
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}

	if err := cmd.Start(); err != nil {
		return 0, "", fmt.Errorf("starting background run: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, logPath, nil
}

func openFlowLog(prefix, flowKey string) (*os.File, string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, "", fmt.Errorf("locating cache dir: %w", err)
	}
	dir := filepath.Join(cache, "flows", "live", "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", fmt.Errorf("creating log dir: %w", err)
	}
	key := flowKey
	if len(key) > 12 {
		key = key[:12]
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s-%d.log", prefix, key, time.Now().UnixNano()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("creating log file: %w", err)
	}
	return f, path, nil
}

func sortDescriptors(descs []live.Descriptor) []live.Descriptor {
	out := append([]live.Descriptor(nil), descs...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAtUnixNano > out[j].CreatedAtUnixNano
	})
	return out
}
