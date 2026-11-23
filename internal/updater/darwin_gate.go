//go:build darwin

package updater

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

// The only IPC operation is asking whether startup may proceed. It cannot
// install, recover, open a path, choose a candidate or execute a command.
type startGate struct{ allow atomic.Bool }

func peerUID(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var credentialErr error
	if err := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		credentialErr = err
		if err == nil {
			uid = cred.Uid
		}
	}); err != nil {
		return 0, err
	}
	return uid, credentialErr
}

func (g *startGate) serve(listener *net.UnixListener, uid uint32) {
	slots := make(chan struct{}, 8)
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = conn.Close()
			continue
		}
		go func() {
			defer func() { _ = conn.Close(); <-slots }()
			if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				return
			}
			peer, err := peerUID(conn)
			if err != nil || peer != uid {
				return
			}
			raw, err := io.ReadAll(io.LimitReader(conn, 9))
			if err != nil || string(raw) != "START\n" {
				return
			}
			reply := "WAIT\n"
			if g.allow.Load() {
				reply = "GO\n"
			}
			_, _ = io.WriteString(conn, reply)
		}()
	}
}

func (g *startGate) listen(uid, gid uint32) (*net.UnixListener, error) {
	if info, err := os.Lstat(macSocket); err == nil {
		meta, _, err := metadata(macSocket, info)
		if err != nil || meta.UID != 0 || info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("refusing to replace an unmanaged startup socket")
		}
		if err := os.Remove(macSocket); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: macSocket, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err := os.Chown(macSocket, 0, int(gid)); err != nil {
		_ = listener.Close()
		return nil, err
	}
	if err := os.Chmod(macSocket, 0660); err != nil { // #nosec G302 -- only root and dedicated _loom may request read-only startup authorization
		_ = listener.Close()
		return nil, err
	}
	go g.serve(listener, uid)
	return listener, nil
}

func queryMacGate(ctx context.Context, path string) error {
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", path)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	c, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("startup requires Unix peer credentials")
	}
	uid, err := peerUID(c)
	if err != nil || uid != 0 {
		return errors.New("startup authorization peer is not root")
	}
	if err := c.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	if _, err := io.WriteString(c, "START\n"); err != nil {
		return err
	}
	if err := c.CloseWrite(); err != nil {
		return err
	}
	reply, err := io.ReadAll(io.LimitReader(c, 16))
	if err != nil || string(reply) != "GO\n" {
		return errors.New("startup awaits native recovery authorization")
	}
	return nil
}

// StartupGuard runs before config writes, SQLite open and migrations. An
// enrolled daemon never relies on launchd job ordering or a stale marker's
// contents to prove recovery completed. A missing/crashed helper fails closed.
func StartupGuard(ctx context.Context) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self != BinDir+"/loom" {
		return nil
	}
	if _, err := os.Lstat(markerPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := nativeInstalled(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for {
		if err := queryMacGate(ctx, macSocket); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("macOS native recovery has not authorized startup; inspect the privileged updater")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Top-level launchctl fields only: nested environment/resource dictionaries
// cannot impersonate security properties. Missing/changed output fails closed.
func macJobFields(raw []byte) (map[string]string, []string, error) {
	fields := map[string]string{}
	var args []string
	inArgs := false
	for _, line := range strings.Split(string(raw), "\n") {
		if inArgs {
			if line == "\t}" {
				inArgs = false
				continue
			}
			if !strings.HasPrefix(line, "\t\t") || strings.HasPrefix(line, "\t\t\t") {
				return nil, nil, errors.New("unexpected launchd arguments")
			}
			args = append(args, strings.TrimPrefix(line, "\t\t"))
			continue
		}
		if !strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "\t\t") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "\t"), " = ")
		if !ok {
			continue
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, nil, errors.New("duplicate launchd property")
		}
		fields[key] = value
		if key == "arguments" && value == "{" {
			inArgs = true
		}
	}
	if inArgs {
		return nil, nil, errors.New("incomplete launchd properties")
	}
	return fields, args, nil
}
