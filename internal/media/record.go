package media

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// VoiceRecorder captures microphone audio to an Opus/OGG file suitable for WhatsApp PTT.
type VoiceRecorder struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	path    string
	started time.Time
	running bool
}

// StartVoiceRecord begins capturing from the default system microphone.
// Linux: PulseAudio/PipeWire · Windows: WASAPI · macOS: AVFoundation.
func StartVoiceRecord(dir string) (*VoiceRecorder, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg no está instalado — hace falta para grabar notas de voz")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("voice_%d.ogg", time.Now().UnixNano()))

	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
	}
	switch runtime.GOOS {
	case "windows":
		args = append(args, "-f", "wasapi", "-i", "default")
	case "darwin":
		args = append(args, "-f", "avfoundation", "-i", ":0")
	default:
		args = append(args, "-f", "pulse", "-i", "default")
	}
	args = append(args,
		"-ac", "1",
		"-ar", "16000",
		"-af", "volume=2.5,highpass=f=80,lowpass=f=8000",
		"-c:a", "libopus",
		"-b:a", "32k",
		"-application", "voip",
		"-f", "ogg",
		path,
	)

	cmd := exec.Command("ffmpeg", args...)
	setRecordProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("iniciar grabación: %w", err)
	}
	return &VoiceRecorder{
		cmd:     cmd,
		path:    path,
		started: time.Now(),
		running: true,
	}, nil
}

// Path returns the destination file (may be incomplete while recording).
func (r *VoiceRecorder) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Elapsed returns how long we've been recording.
func (r *VoiceRecorder) Elapsed() time.Duration {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started.IsZero() {
		return 0
	}
	return time.Since(r.started)
}

// Running reports whether ffmpeg is still capturing.
func (r *VoiceRecorder) Running() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Stop ends capture cleanly and returns the OGG path + duration in seconds.
func (r *VoiceRecorder) Stop() (path string, seconds uint32, err error) {
	if r == nil {
		return "", 0, fmt.Errorf("no hay grabación")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return "", 0, fmt.Errorf("grabación ya detenida")
	}
	r.running = false
	secs := uint32(time.Since(r.started).Seconds() + 0.5)
	if secs == 0 {
		secs = 1
	}
	path = r.path

	// SIGINT lets ffmpeg finalize the OGG container.
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- r.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = r.cmd.Process.Kill()
			<-done
		}
	}

	st, statErr := os.Stat(path)
	if statErr != nil {
		return "", 0, fmt.Errorf("archivo de voz: %w", statErr)
	}
	if st.Size() < 64 {
		_ = os.Remove(path)
		return "", 0, fmt.Errorf("grabación vacía (¿micrófono permitido?)")
	}
	return path, secs, nil
}

// Cancel aborts recording and deletes the temp file.
func (r *VoiceRecorder) Cancel() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running && r.cmd != nil && r.cmd.Process != nil {
		r.running = false
		_ = r.cmd.Process.Kill()
		_, _ = r.cmd.Process.Wait()
	}
	if r.path != "" {
		_ = os.Remove(r.path)
	}
}
