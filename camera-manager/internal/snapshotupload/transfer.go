package snapshotupload

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Network connections share a deadline covering dial, authentication and data
// transfer. Cancellation also interrupts stalled protocol reads immediately.
type contextConn struct {
	net.Conn
	stop func() bool
}

func (c *contextConn) Close() error { c.stop(); return c.Conn.Close() }

func dial(ctx context.Context, network, address string) (net.Conn, error) {
	c, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if ok {
		_ = c.SetDeadline(deadline)
	}
	return &contextConn{Conn: c, stop: context.AfterFunc(ctx, func() { _ = c.Close() })}, nil
}

func transfer(ctx context.Context, cfg Config, password, filename string, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var err error
	if cfg.Protocol == "ftp" {
		err = transferFTP(ctx, cfg, password, filename, data)
	} else {
		err = transferSFTP(ctx, cfg, password, filename, data)
	}
	if ctx.Err() != nil {
		return errors.New("Upload abgebrochen oder Zeitlimit von 30 Sekunden erreicht. Der Server kann eine unvollständige .part-Datei enthalten.")
	}
	return err
}

func transferFTP(ctx context.Context, cfg Config, password, filename string, data []byte) error {
	c, err := ftp.Dial(net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), ftp.DialWithContext(ctx), ftp.DialWithDialFunc(func(network, address string) (net.Conn, error) { return dial(ctx, network, address) }))
	if err != nil {
		return errors.New("FTP-Server nicht erreichbar. Server, Port und Netzwerk prüfen.")
	}
	defer c.Quit()
	if err := c.Login(cfg.Username, password); err != nil {
		return errors.New("FTP-Anmeldung fehlgeschlagen. Benutzername und Passwort prüfen.")
	}
	if err := c.ChangeDir(cfg.Directory); err != nil {
		return errors.New("FTP-Zielverzeichnis nicht zugänglich. Verzeichnis und Berechtigungen prüfen.")
	}
	temp := filename + "." + uuid.NewString() + ".part"
	if err := c.Stor(temp, bytes.NewReader(data)); err != nil {
		_ = c.Delete(temp)
		return errors.New("FTP-Upload fehlgeschlagen. Schreibrechte und freien Speicher prüfen.")
	}
	if err := c.Rename(temp, filename); err != nil {
		_ = c.Delete(temp)
		return errors.New("FTP-Bild konnte nicht fertiggestellt werden. Der Server muss das Umbenennen und bei festem Dateinamen das Ersetzen vorhandener Dateien erlauben.")
	}
	return nil
}

var errHostKey = errors.New("SFTP-Hostschlüssel stimmt nicht mit dem gespeicherten Fingerabdruck überein. Fingerabdruck beim Serverbetreiber prüfen.")

func transferSFTP(ctx context.Context, cfg Config, password, filename string, data []byte) error {
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := dial(ctx, "tcp", address)
	if err != nil {
		return errors.New("SFTP-Server nicht erreichbar. Server, Port und Netzwerk prüfen.")
	}
	defer conn.Close()
	config := &ssh.ClientConfig{User: cfg.Username, Auth: []ssh.AuthMethod{ssh.Password(password)}, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if subtle.ConstantTimeCompare([]byte(ssh.FingerprintSHA256(key)), []byte(cfg.HostKey)) != 1 {
			return errHostKey
		}
		return nil
	}}
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		if errors.Is(err, errHostKey) {
			return errHostKey
		}
		return errors.New("SFTP-Verbindung oder Anmeldung fehlgeschlagen. SSH-Dienst, Benutzername und Passwort prüfen.")
	}
	client := ssh.NewClient(clientConn, chans, reqs)
	defer client.Close()
	c, err := sftp.NewClient(client)
	if err != nil {
		return errors.New("Der SSH-Server stellt keinen erreichbaren SFTP-Dienst bereit.")
	}
	defer c.Close()
	target := path.Join(cfg.Directory, filename)
	temp := target + "." + uuid.NewString() + ".part"
	f, err := c.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return errors.New("SFTP-Zieldatei kann nicht angelegt werden. Verzeichnis, Schreibrechte und freien Speicher prüfen.")
	}
	_, writeErr := io.Copy(f, bytes.NewReader(data))
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = c.Remove(temp)
		return errors.New("SFTP-Upload fehlgeschlagen. Schreibrechte und freien Speicher prüfen.")
	}
	// Standard SFTP v3 rename rejects an existing destination. OpenSSH's
	// extension publishes the completed file with POSIX replacement semantics.
	// Never remove the old image first: a failed rename must not lose it.
	if _, ok := c.HasExtension("posix-rename@openssh.com"); ok {
		err = c.PosixRename(temp, target)
	} else {
		err = c.Rename(temp, target)
	}
	if err != nil {
		_ = c.Remove(temp)
		return errors.New("SFTP-Bild konnte nicht fertiggestellt werden. Der Server muss das Umbenennen erlauben; zum sicheren Ersetzen vorhandener Bilder wird posix-rename@openssh.com benötigt.")
	}
	return nil
}
