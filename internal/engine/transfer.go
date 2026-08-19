package engine

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/models"
	"github.com/schollz/croc/v10/src/utils"
)

// defaultRelayOptions fills in the relay address/password croc's own CLI
// defaults to (models.DEFAULT_RELAY / DEFAULT_PASSPHRASE) — these are
// only defaulted at the CLI's flag-parsing layer, not inside the croc
// library itself, so a library caller that leaves them unset gets an
// empty RelayAddress and fails to connect to anything. Confirmed by
// reading cli.go directly rather than assuming the library defaults
// these on its own.
func defaultRelayOptions() croc.Options {
	return croc.Options{
		RelayAddress:  models.DEFAULT_RELAY,
		RelayAddress6: models.DEFAULT_RELAY6,
		RelayPassword: models.DEFAULT_PASSPHRASE,
		// setupLocalRelay (called unconditionally inside Send, for the
		// separate same-network fast path croc also tries) indexes
		// RelayPorts[0] with no length check — leaving this unset crashes
		// with an index-out-of-range panic, confirmed by hitting it
		// directly. Same 5-port range croc's own CLI defaults to.
		RelayPorts: []string{"9009", "9010", "9011", "9012", "9013"},
		// Same story as RelayAddress/RelayPorts: only defaulted at the
		// CLI's flag-parsing layer ("no such curve" otherwise).
		Curve:         "p256",
		HashAlgorithm: "xxhash",
	}
}

// TransferProgress is streamed to the caller-supplied callback during
// SendOverInternet/ReceiveFromInternet.
type TransferProgress struct {
	Stage   string // waiting, transferring, done, error
	Percent float64
}

// NewTransferCode returns a fresh, human-readable code phrase — the same
// format croc's own CLI generates and accepts, so someone on the other
// end can receive with the real `croc <code>` command even without this
// app installed.
func NewTransferCode() string {
	return utils.GetRandomName()
}

// receiveMu serializes Receive calls: croc's library has no per-call
// output-directory parameter, so writing to a chosen folder means
// os.Chdir-ing the whole process before the call and back after — this
// is the exact pattern croc's own CLI (src/cli/cli.go) and its own test
// suite use, not a workaround. Since os.Chdir is process-wide, not
// per-goroutine, only one receive can safely be in flight at a time.
var receiveMu sync.Mutex

// SendOverInternet shares paths (absolute paths; individual files or one
// folder) under code, relayed through croc's relay — which only ever
// sees end-to-end-encrypted ciphertext, never file contents — until
// someone on the other end receives with the same code or ctx is
// canceled.
func SendOverInternet(ctx context.Context, paths []string, code string, onProgress func(TransferProgress)) error {
	filesInfo, emptyFolders, totalFolders, err := croc.GetFilesInfo(paths, false, false, nil)
	if err != nil {
		return fmt.Errorf("preparing files: %w", err)
	}

	opts := defaultRelayOptions()
	opts.IsSender = true
	opts.SharedSecret = code
	opts.NoPrompt = true
	opts.DisableClipboard = true

	c, err := croc.NewCtx(ctx, opts)
	if err != nil {
		return fmt.Errorf("starting transfer: %w", err)
	}

	stop := watchProgress(ctx, c, onProgress)
	defer stop()

	return c.Send(filesInfo, emptyFolders, totalFolders)
}

// ReceiveFromInternet receives whatever was sent under code into
// outputDir.
func ReceiveFromInternet(ctx context.Context, code, outputDir string, onProgress func(TransferProgress)) error {
	receiveMu.Lock()
	defer receiveMu.Unlock()

	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reading current directory: %w", err)
	}
	if err := os.Chdir(outputDir); err != nil {
		return fmt.Errorf("switching to output directory: %w", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	opts := defaultRelayOptions()
	opts.IsSender = false
	opts.SharedSecret = code
	opts.NoPrompt = true
	opts.Overwrite = true
	opts.DisableClipboard = true

	c, err := croc.NewCtx(ctx, opts)
	if err != nil {
		return fmt.Errorf("starting transfer: %w", err)
	}

	stop := watchProgress(ctx, c, onProgress)
	defer stop()

	return c.Receive()
}

// watchProgress polls the client's exported state — croc's library has
// no progress callback, only fields a running CLI would render into its
// own progress bar — and republishes it as TransferProgress on a fixed
// cadence until the returned stop func is called.
func watchProgress(ctx context.Context, c *croc.Client, onProgress func(TransferProgress)) func() {
	if onProgress == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				onProgress(progressFromClient(c))
			}
		}
	}()
	return func() { close(done) }
}

func progressFromClient(c *croc.Client) TransferProgress {
	if c.SuccessfulTransfer {
		return TransferProgress{Stage: "done", Percent: 100}
	}
	if !c.Step1ChannelSecured {
		return TransferProgress{Stage: "waiting"}
	}
	var total int64
	for _, f := range c.FilesToTransfer {
		total += f.Size
	}
	pct := 0.0
	if total > 0 {
		pct = float64(c.TotalSent) / float64(total) * 100
	}
	return TransferProgress{Stage: "transferring", Percent: pct}
}
