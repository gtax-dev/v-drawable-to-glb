package main

import (
	"bytes"
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
	exitCode := run()
	os.Exit(exitCode)
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

	inputData, err := os.ReadFile(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: read input: %v\n", err)
		return 1
	}

	var ytdData []byte
	if strings.TrimSpace(*ytdPath) != "" {
		ytdData, err = os.ReadFile(*ytdPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: read ytd: %v\n", err)
			return 1
		}
	}

	body, contentType, err := buildMultipart(ext, *inputPath, inputData, *ytdPath, ytdData, *name, *lod, *drawable, *drawableIndex, *rotationXDeg, *rotationYDeg, *rotationZDeg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: build request: %v\n", err)
		return 1
	}

	resp, err := postConvert(defaultPublicAPI, endpoint, body, contentType, key, &http.Client{Timeout: *timeout})
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

// postConvert sends a multipart conversion request. Caller must close resp.Body.
func postConvert(resolvedBase, endpoint string, body []byte, contentType, apiKey string, client *http.Client) (*http.Response, error) {
	urlStr := strings.TrimRight(resolvedBase, "/") + endpoint
	req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return client.Do(req)
}

func buildMultipart(
	ext, inputPath string,
	inputData []byte,
	ytdPath string,
	ytdData []byte,
	name, lod, drawable string,
	drawableIndex int,
	rotationXDeg, rotationYDeg, rotationZDeg float64,
) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	switch ext {
	case ".ydr":
		if err := writeFormFile(w, "ydr", filepath.Base(inputPath), inputData); err != nil {
			return nil, "", err
		}
	case ".ydd":
		if err := writeFormFile(w, "ydd", filepath.Base(inputPath), inputData); err != nil {
			return nil, "", err
		}
	default:
		return nil, "", fmt.Errorf("unsupported extension %q", ext)
	}

	if len(ytdData) > 0 {
		filename := "textures.ytd"
		if strings.TrimSpace(ytdPath) != "" {
			filename = filepath.Base(ytdPath)
		}
		if err := writeFormFile(w, "ytd", filename, ytdData); err != nil {
			return nil, "", err
		}
	}

	if strings.TrimSpace(name) != "" {
		if err := w.WriteField("name", name); err != nil {
			return nil, "", err
		}
	}
	if strings.TrimSpace(lod) != "" {
		if err := w.WriteField("lod", lod); err != nil {
			return nil, "", err
		}
	}
	if ext == ".ydd" {
		if strings.TrimSpace(drawable) != "" {
			if err := w.WriteField("drawable", drawable); err != nil {
				return nil, "", err
			}
		}
		if drawableIndex >= 0 {
			if err := w.WriteField("drawableIndex", fmt.Sprintf("%d", drawableIndex)); err != nil {
				return nil, "", err
			}
		}
	}

	if rotationXDeg != 0 {
		if err := w.WriteField("rotationX", fmt.Sprintf("%g", rotationXDeg)); err != nil {
			return nil, "", err
		}
	}
	if rotationYDeg != 0 {
		if err := w.WriteField("rotationY", fmt.Sprintf("%g", rotationYDeg)); err != nil {
			return nil, "", err
		}
	}
	if rotationZDeg != 0 {
		if err := w.WriteField("rotationZ", fmt.Sprintf("%g", rotationZDeg)); err != nil {
			return nil, "", err
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func writeFormFile(w *multipart.Writer, fieldName, filename string, data []byte) error {
	part, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
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
