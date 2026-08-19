// Package engine provides two ways to share files, matching the two
// distinct jobs "Private File Share" actually does: a local HTTP server
// on this Wi-Fi (for a nearby phone or another computer — no account, no
// internet, nothing leaves the network) and an internet transfer via croc
// (for anywhere — end-to-end encrypted through croc's relay, which never
// sees plaintext content, only ciphertext).
package engine

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/skip2/go-qrcode"
)

// SharedItem is one file being served by a LocalShare.
type SharedItem struct {
	Path string
	Name string
	Size int64
}

// LocalShare is a running local HTTP server exposing one or more files
// (or one folder) to anything on the same network — a phone's browser,
// another computer — with no account and no internet round trip.
type LocalShare struct {
	URL string

	listener  net.Listener
	server    *http.Server
	items     []SharedItem
	folderDir string // set instead of items when sharing a whole folder

	downloads atomic.Int64
}

// LocalIP returns this machine's LAN-reachable IPv4 address — the one
// other devices on the same Wi-Fi can actually reach, as opposed to
// 127.0.0.1 which only this machine can.
func LocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("reading network interfaces: %w", err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			return ip4.String(), nil
		}
	}
	return "", fmt.Errorf("no network connection found — connect to Wi-Fi first")
}

// StartLocalShare serves paths — a single folder, or one or more
// individual files — over HTTP on the LAN. A lone folder is served
// browsable (subfolders and all); individual files get a small
// auto-generated download page.
func StartLocalShare(paths []string) (*LocalShare, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("nothing to share")
	}

	ip, err := LocalIP()
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", ip+":0")
	if err != nil {
		return nil, fmt.Errorf("starting local share server: %w", err)
	}

	s := &LocalShare{listener: ln, URL: "http://" + ln.Addr().String() + "/"}
	mux := http.NewServeMux()

	if len(paths) == 1 {
		info, err := os.Stat(paths[0])
		if err != nil {
			_ = ln.Close()
			return nil, fmt.Errorf("reading %s: %w", filepath.Base(paths[0]), err)
		}
		if info.IsDir() {
			s.folderDir = paths[0]
			mux.Handle("GET /", s.trackDownloads(http.FileServer(http.Dir(s.folderDir))))
			s.server = &http.Server{Handler: mux}
			go s.server.Serve(ln)
			return s, nil
		}
	}

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			_ = ln.Close()
			return nil, fmt.Errorf("reading %s: %w", filepath.Base(p), err)
		}
		if info.IsDir() {
			_ = ln.Close()
			return nil, fmt.Errorf("%s is a folder — share it on its own, not mixed with individual files", filepath.Base(p))
		}
		s.items = append(s.items, SharedItem{Path: p, Name: filepath.Base(p), Size: info.Size()})
	}

	if len(s.items) == 1 {
		mux.HandleFunc("GET /", s.trackDownloadsFunc(s.serveSingle))
	} else {
		mux.HandleFunc("GET /", s.serveIndex)
		mux.HandleFunc("GET /files/{name}", s.trackDownloadsFunc(s.serveNamed))
	}
	s.server = &http.Server{Handler: mux}
	go s.server.Serve(ln)
	return s, nil
}

// Stop closes the listener — any in-flight download is cut off, same as
// unplugging a real hotspot.
func (s *LocalShare) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// Downloads is how many times something has actually been fetched, not
// how many times the page was loaded.
func (s *LocalShare) Downloads() int64 {
	return s.downloads.Load()
}

// QRCodePNG renders this share's URL as a scannable QR code.
func (s *LocalShare) QRCodePNG() ([]byte, error) {
	return qrcode.Encode(s.URL, qrcode.Medium, 240)
}

func (s *LocalShare) trackDownloads(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.downloads.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (s *LocalShare) trackDownloadsFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.downloads.Add(1)
		next(w, r)
	}
}

func (s *LocalShare) serveSingle(w http.ResponseWriter, r *http.Request) {
	item := s.items[0]
	w.Header().Set("Content-Disposition", `attachment; filename="`+item.Name+`"`)
	http.ServeFile(w, r, item.Path)
}

func (s *LocalShare) serveNamed(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	for _, item := range s.items {
		if item.Name == name {
			w.Header().Set("Content-Disposition", `attachment; filename="`+item.Name+`"`)
			http.ServeFile(w, r, item.Path)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *LocalShare) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><meta name="viewport" content="width=device-width, initial-scale=1"><body style="font:16px -apple-system,sans-serif;max-width:480px;margin:40px auto;padding:0 16px"><h1 style="font-size:18px">Shared files</h1><ul style="padding-left:20px">`)
	for _, item := range s.items {
		fmt.Fprintf(w, `<li style="margin:10px 0"><a href="/files/%s">%s</a> — %s</li>`, item.Name, item.Name, byteSize(item.Size))
	}
	fmt.Fprint(w, `</ul></body>`)
}

func byteSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
