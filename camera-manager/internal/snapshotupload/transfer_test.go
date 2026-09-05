package snapshotupload

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// These protocol fixtures listen exclusively on loopback and use generated
// host keys, temporary files and synthetic credentials. No appliance is used.
func ftpServer(t *testing.T, reject string, initial ...map[string][]byte) (Config, <-chan map[string][]byte) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	files := make(chan map[string][]byte, 1)
	go func() {
		stored := map[string][]byte{}
		for _, files := range initial {
			for name, data := range files {
				stored[name] = bytes.Clone(data)
			}
		}
		defer func() { files <- stored }()
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprint(conn, "220 local test FTP\r\n")
		scanner := bufio.NewScanner(conn)
		var data net.Listener
		var rename string
		defer func() {
			if data != nil {
				data.Close()
			}
		}()
		for scanner.Scan() {
			verb, arg, _ := strings.Cut(scanner.Text(), " ")
			switch verb {
			case "USER":
				fmt.Fprint(conn, "331 password required\r\n")
			case "PASS":
				if arg != "local-test-password" || reject == "auth" {
					fmt.Fprint(conn, "530 local-test-password rejected\r\n")
				} else {
					fmt.Fprint(conn, "230 logged in\r\n")
				}
			case "FEAT":
				fmt.Fprint(conn, "211 End\r\n")
			case "TYPE", "OPTS":
				fmt.Fprint(conn, "200 OK\r\n")
			case "CWD":
				if reject == "directory" {
					fmt.Fprint(conn, "550 no such directory\r\n")
				} else {
					fmt.Fprint(conn, "250 changed directory\r\n")
				}
			case "EPSV":
				data, err = net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					return
				}
				_ = data.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
				fmt.Fprintf(conn, "229 Entering Extended Passive Mode (|||%d|)\r\n", data.Addr().(*net.TCPAddr).Port)
			case "STOR":
				if reject == "write" {
					fmt.Fprint(conn, "550 permission denied\r\n")
					continue
				}
				fmt.Fprint(conn, "150 receiving\r\n")
				dc, err := data.Accept()
				if err != nil {
					return
				}
				_ = dc.SetDeadline(time.Now().Add(5 * time.Second))
				stored[arg], _ = io.ReadAll(dc)
				dc.Close()
				data.Close()
				fmt.Fprint(conn, "226 complete\r\n")
			case "RNFR":
				rename = arg
				fmt.Fprint(conn, "350 rename ready\r\n")
			case "RNTO":
				if reject == "rename" {
					fmt.Fprint(conn, "550 rename denied\r\n")
					continue
				}
				stored[arg] = stored[rename]
				delete(stored, rename)
				fmt.Fprint(conn, "250 renamed\r\n")
			case "DELE":
				delete(stored, arg)
				fmt.Fprint(conn, "250 deleted\r\n")
			case "QUIT":
				fmt.Fprint(conn, "221 goodbye\r\n")
				return
			default:
				fmt.Fprint(conn, "502 unsupported\r\n")
			}
		}
	}()
	c := testConfig()
	c.Host = "127.0.0.1"
	c.Port = l.Addr().(*net.TCPAddr).Port
	return c, files
}

func TestFTPTransfersJPEGAndCleansFailedUploads(t *testing.T) {
	for _, reject := range []string{"", "auth", "directory", "write", "rename"} {
		t.Run("reject_"+reject, func(t *testing.T) {
			cfg, result := ftpServer(t, reject)
			data := testJPEG(t)
			err := transfer(context.Background(), cfg, "local-test-password", "test.jpg", data)
			if (reject == "") != (err == nil) {
				t.Fatalf("unexpected result: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "local-test-password") {
				t.Fatal("server reply exposed password")
			}
			select {
			case files := <-result:
				if reject == "" && !bytes.Equal(files["test.jpg"], data) {
					t.Fatal("FTP did not transfer original JPEG")
				}
				if reject != "" && len(files) != 0 {
					t.Fatal("failed upload left a published or temporary image")
				}
			case <-time.After(6 * time.Second):
				t.Fatal("FTP fixture did not finish")
			}
		})
	}
}

func sftpServer(t *testing.T, handlers ...sftp.Handlers) (Config, string, *atomic.Int32) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	auths := &atomic.Int32{}
	serverConfig := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		auths.Add(1)
		if c.User() != "test-user" || string(p) != "local-test-password" {
			return nil, errors.New("local-test-password rejected")
		}
		return nil, nil
	}}
	serverConfig.AddHostKey(signer)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	done := make(chan struct{})
	t.Cleanup(func() {
		l.Close()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("SFTP fixture did not finish")
		}
	})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		sc, chans, requests, err := ssh.NewServerConn(conn, serverConfig)
		if err != nil {
			return
		}
		defer sc.Close()
		go ssh.DiscardRequests(requests)
		for ch := range chans {
			if ch.ChannelType() != "session" {
				ch.Reject(ssh.UnknownChannelType, "session only")
				continue
			}
			channel, reqs, err := ch.Accept()
			if err != nil {
				return
			}
			for req := range reqs {
				var subsystem struct{ Name string }
				if req.Type != "subsystem" || ssh.Unmarshal(req.Payload, &subsystem) != nil || subsystem.Name != "sftp" {
					req.Reply(false, nil)
					continue
				}
				req.Reply(true, nil)
				if len(handlers) > 0 {
					server := sftp.NewRequestServer(channel, handlers[0])
					_ = server.Serve()
					server.Close()
					channel.Close()
					break
				}
				server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(dir))
				if err != nil {
					channel.Close()
					return
				}
				_ = server.Serve()
				server.Close()
				channel.Close()
				break
			}
		}
	}()
	c := Config{Protocol: "sftp", Host: "127.0.0.1", Port: l.Addr().(*net.TCPAddr).Port, Username: "test-user", Directory: ".", HostKey: ssh.FingerprintSHA256(signer.PublicKey())}
	return c, dir, auths
}

func TestSFTPTransfersAndRejectsWrongHostBeforeAuthentication(t *testing.T) {
	for _, mode := range []string{"success", "host-key", "password", "directory"} {
		t.Run(mode, func(t *testing.T) {
			cfg, dir, auths := sftpServer(t)
			password := "local-test-password"
			if mode == "host-key" {
				cfg.HostKey = "SHA256:wrong"
			}
			if mode == "password" {
				password = "wrong-test-password"
			}
			if mode == "directory" {
				cfg.Directory = "missing/directory"
			}
			data := testJPEG(t)
			err := transfer(context.Background(), cfg, password, "test.jpg", data)
			if (mode == "success") != (err == nil) {
				t.Fatalf("unexpected result: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "test-password") {
				t.Fatal("SFTP error leaked a credential")
			}
			if mode == "host-key" && (!errors.Is(err, errHostKey) || auths.Load() != 0) {
				t.Fatalf("untrusted server received authentication: %v, count %d", err, auths.Load())
			}
			files, _ := os.ReadDir(dir)
			if mode != "success" && len(files) != 0 {
				t.Fatal("failed SFTP upload created a file")
			}
			if mode == "success" {
				got, err := os.ReadFile(filepath.Join(dir, "test.jpg"))
				if err != nil || !bytes.Equal(got, data) || len(files) != 1 {
					t.Fatalf("SFTP transfer content mismatch: %v", err)
				}
			}
		})
	}
}

func TestCancellationInterruptsStalledProtocolHandshake(t *testing.T) {
	for _, protocol := range []string{"ftp", "sftp"} {
		t.Run(protocol, func(t *testing.T) {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				c, err := l.Accept()
				if err == nil {
					accepted <- c
				}
			}()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			host, port, _ := net.SplitHostPort(l.Addr().String())
			p, _ := strconv.Atoi(port)
			cfg := testConfig()
			cfg.Protocol = protocol
			cfg.Host = host
			cfg.Port = p
			finished := make(chan error, 1)
			go func() { finished <- transfer(ctx, cfg, "test", "test.jpg", []byte("test")) }()
			select {
			case c := <-accepted:
				defer c.Close()
			case <-time.After(time.Second):
				t.Fatal("did not connect")
			}
			cancel()
			select {
			case err := <-finished:
				if err == nil {
					t.Fatal("cancelled transfer succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation did not interrupt handshake")
			}
		})
	}
}
