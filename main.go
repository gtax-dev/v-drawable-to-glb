package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultPublicAPI = "https://public-drawable-to-glb.gtax.dev"
	apiKeyEnv        = "V_DRAWABLE_TO_GLB_API_KEY"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -i <file.ydr|file.ydd> [options]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Convert a GTA V drawable (.ydr/.ydd) to GLB via HTTP multipart POST.")
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}
	os.Exit(run())
}

func run() int {
	var (
		inputPath     = flag.String("i", "", "input `.ydr` or `.ydd` file (required)")
		outputPath    = flag.String("o", "", "output `.glb` path (default: input basename with .glb)")
		ytdPath       = flag.String("ytd", "", "optional `.ytd` texture dictionary")
		name          = flag.String("name", "", "optional output filename stem for the API")
		lod           = flag.String("lod", "", "optional LOD: high, medium, low, verylow")
		drawable      = flag.String("drawable", "", "optional YDD drawable name selector")
		drawableIndex = flag.Int("drawable-index", -1, "optional zero-based YDD drawable index (-1 = omit)")
		rotationXDeg  = flag.Float64("rotation-x", 0, "optional degrees about world +X (root node; order X, then Y, then Z)")
		rotationYDeg  = flag.Float64("rotation-y", 0, "optional degrees about world +Y")
		rotationZDeg  = flag.Float64("rotation-z", 0, "optional degrees about world +Z")
		apiKey        = flag.String("api-key", "", "API key (or env "+apiKeyEnv+")")
		timeout       = flag.Duration("timeout", 30*time.Minute, "HTTP client timeout")
	)
	flag.Parse()

	if strings.TrimSpace(*inputPath) == "" {
		fmt.Fprintln(os.Stderr, "error: -i is required")
		flag.Usage()
		return 2
	}

	ext := strings.ToLower(filepath.Ext(*inputPath))
	var endpoint string
	switch ext {
	case ".ydr":
		endpoint = "/convert/ydr-to-glb"
	case ".ydd":
		endpoint = "/convert/ydd-to-glb"
	default:
		fmt.Fprintf(os.Stderr, "error: input must be .ydr or .ydd, got %q\n", ext)
		return 2
	}

	key := strings.TrimSpace(*apiKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv(apiKeyEnv))
	}

	out := *outputPath
	if out == "" {
		base := strings.TrimSuffix(filepath.Base(*inputPath), ext)
		out = base + ".glb"
	}

	// Open files eagerly so we detect missing files before starting the request.
	inputFile, err := os.Open(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: read input: %v\n", err)
		return 1
	}
	defer inputFile.Close()

	var ytdFile *os.File
	if p := strings.TrimSpace(*ytdPath); p != "" {
		ytdFile, err = os.Open(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: read ytd: %v\n", err)
			return 1
		}
		defer ytdFile.Close()
	}

	fieldName := strings.TrimPrefix(ext, ".")
	resp, err := postConvertStream(
		defaultPublicAPI, endpoint,
		func(mw *multipart.Writer) error {
			part, err := mw.CreateFormFile(fieldName, filepath.Base(*inputPath))
			if err != nil {
				return err
			}
			if _, err := io.Copy(part, inputFile); err != nil {
				return fmt.Errorf("read input: %w", err)
			}
			if ytdFile != nil {
				part, err := mw.CreateFormFile("ytd", filepath.Base(*ytdPath))
				if err != nil {
					return err
				}
				if _, err := io.Copy(part, ytdFile); err != nil {
					return fmt.Errorf("read ytd: %w", err)
				}
			}
			if strings.TrimSpace(*name) != "" {
				if err := mw.WriteField("name", *name); err != nil {
					return err
				}
			}
			if strings.TrimSpace(*lod) != "" {
				if err := mw.WriteField("lod", *lod); err != nil {
					return err
				}
			}
			if ext == ".ydd" {
				if strings.TrimSpace(*drawable) != "" {
					if err := mw.WriteField("drawable", *drawable); err != nil {
						return err
					}
				}
				if *drawableIndex >= 0 {
					if err := mw.WriteField("drawableIndex", fmt.Sprintf("%d", *drawableIndex)); err != nil {
						return err
					}
				}
			}
			if *rotationXDeg != 0 {
				if err := mw.WriteField("rotationX", fmt.Sprintf("%g", *rotationXDeg)); err != nil {
					return err
				}
			}
			if *rotationYDeg != 0 {
				if err := mw.WriteField("rotationY", fmt.Sprintf("%g", *rotationYDeg)); err != nil {
					return err
				}
			}
			if *rotationZDeg != 0 {
				if err := mw.WriteField("rotationZ", fmt.Sprintf("%g", *rotationZDeg)); err != nil {
					return err
				}
			}
			return nil
		},
		key,
		&http.Client{Timeout: *timeout},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: http: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	printStats(os.Stderr, resp.Header)

	if resp.StatusCode != http.StatusOK {
		msg := readErrorBody(resp)
		fmt.Fprintf(os.Stderr, "error: %s %s\n", resp.Status, msg)
		return 1
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "model/gltf-binary") {
		msg := readErrorBody(resp)
		fmt.Fprintf(os.Stderr, "error: unexpected Content-Type %q: %s\n", ct, msg)
		return 1
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: read body: %v\n", err)
		return 1
	}

	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: write output: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", out, len(data))
	return 0
}

// postConvertStream sends a multipart POST with a streaming body written by writeBody.
// Files are piped directly from disk — no full-body buffer in RAM.
func postConvertStream(
	resolvedBase, endpoint string,
	writeBody func(mw *multipart.Writer) error,
	apiKey string,
	client *http.Client,
) (*http.Response, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		err := writeBody(mw)
		if closeErr := mw.Close(); err == nil {
			err = closeErr
		}
		pw.CloseWithError(err)
	}()

	urlStr := strings.TrimRight(resolvedBase, "/") + endpoint
	req, err := http.NewRequest(http.MethodPost, urlStr, pr)
	if err != nil {
		_ = pr.CloseWithError(err)
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return client.Do(req)
}

type apiError struct {
	Error string `json:"error"`
}

func readErrorBody(resp *http.Response) string {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("(read body: %v)", err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return ""
	}
	var e apiError
	if json.Unmarshal(b, &e) == nil && e.Error != "" {
		return e.Error
	}
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

func printStats(w io.Writer, h http.Header) {
	stats := []struct {
		label string
		key   string
	}{
		{"geometry", "X-Geometry-Count"},
		{"vertices", "X-Total-Vertices"},
		{"triangles", "X-Total-Triangles"},
		{"lod", "X-LOD-Used"},
		{"textures", "X-Texture-Count"},
		{"queue_wait_ms", "X-Queue-Wait-Ms"},
		{"rate_remaining", "X-RateLimit-Remaining"},
		{"rate_limit", "X-RateLimit-Limit"},
	}
	var parts []string
	for _, s := range stats {
		if v := h.Get(s.key); v != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", s.label, v))
		}
	}
	if warn := h.Get("X-Conversion-Warnings"); warn != "" {
		parts = append(parts, "warnings="+warn)
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, "%s\n", strings.Join(parts, " "))
	}
}
