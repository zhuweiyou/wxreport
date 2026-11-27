// Package shot 把自包含 HTML 截图成移动端长图。
//
// 不启动本地 server、不依赖登录态：交给 headless Chrome 以固定 440 CSS px 视口截图，
// 按 2x/3x 输出 PNG。报告很长时自动分片截图再拼接，绕开 Chrome 单张截图的纹理高度上限。
package shot

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const (
	// DefaultWidth 是移动端 CSS 视口宽度；440 × 2 = 880，正好是手机 2x 图。
	DefaultWidth = 440
	// DefaultScale 是默认缩放倍数。3x 会得到 1125 宽的图，体积更大。
	DefaultScale = 2
	// sliceCSSHeight 是单次截图的 CSS 高度。2x 时 3000 CSS px = 6000 设备 px，
	// 3x 时 9000，都低于 Chrome 的纹理上限（通常 16384）。
	sliceCSSHeight = 3000
)

// Options 控制截图行为。
type Options struct {
	HTML       []byte
	Slug       string
	Width      int
	Scale      int
	ChromePath string
}

// Result 是截图结果。
type Result struct {
	Slug   string
	PNG    []byte
	Width  int
	Height int
}

// Capture 把自包含 HTML 截成整页 PNG。
func Capture(ctx context.Context, opts Options) (Result, error) {
	width := opts.Width
	if width <= 0 {
		width = DefaultWidth
	}
	scale := opts.Scale
	if scale <= 0 {
		scale = DefaultScale
	}
	if scale > 3 {
		return Result{}, fmt.Errorf("scale 最大为 3，当前 %d", scale)
	}
	if len(opts.HTML) == 0 {
		return Result{}, fmt.Errorf("HTML 不能为空")
	}
	slug := opts.Slug
	if slug == "" {
		slug = "report"
	}

	chromePath, err := FindChrome(opts.ChromePath)
	if err != nil {
		return Result{}, err
	}

	allocOpts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocOpts = append(allocOpts,
		chromedp.ExecPath(chromePath),
		chromedp.DisableGPU,
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("force-color-profile", "srgb"),
	)
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		allocOpts = append(allocOpts, chromedp.NoSandbox)
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	// 用 data URL 加载，避免临时文件路径在 Windows/macOS 上的差异。
	dataURL := "data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString(opts.HTML)
	if err := chromedp.Do(browserCtx,
		chromedp.EmulateViewport(int64(width), 900),
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible(chromedp.CSS(`main.page`)),
	); err != nil {
		return Result{}, fmt.Errorf("打开报告页 %s: %w", slug, err)
	}

	// 样式已内嵌，这里只等字体就绪，避免标题/正文用回退字体。
	if _, err := chromedp.Run(browserCtx, chromedp.Poll[bool](
		`!document.fonts || document.fonts.status === 'loaded'`,
		chromedp.WithPollingTimeout(10*time.Second),
	)); err != nil {
		fmt.Fprintln(os.Stderr, "警告: 字体未在 10 秒内就绪，截图可能使用回退字体")
	}

	height, err := chromedp.Run(browserCtx, chromedp.Evaluate[float64](`(() => {
		window.scrollTo(0, 0);
		return Math.max(
			document.documentElement.scrollHeight,
			document.body ? document.body.scrollHeight : 0
		);
	})()`))
	if err != nil {
		return Result{}, fmt.Errorf("计算页面高度: %w", err)
	}
	totalHeight := int(math.Ceil(height))
	if totalHeight <= 0 {
		return Result{}, fmt.Errorf("页面高度为 0，报告可能没有内容")
	}

	out := image.NewRGBA(image.Rect(0, 0, width*scale, totalHeight*scale))
	for y := 0; y < totalHeight; y += sliceCSSHeight {
		h := min(sliceCSSHeight, totalHeight-y)
		chunk, err := captureSlice(browserCtx, width, y, h, scale)
		if err != nil {
			return Result{}, fmt.Errorf("截图第 %d–%d px: %w", y, y+h, err)
		}
		block, err := png.Decode(bytes.NewReader(chunk))
		if err != nil {
			return Result{}, fmt.Errorf("解码截图分片: %w", err)
		}
		wantW, wantH := width*scale, h*scale
		if block.Bounds().Dx() != wantW || block.Bounds().Dy() != wantH {
			return Result{}, fmt.Errorf(
				"截图尺寸异常: 期望 %d×%d，实际 %d×%d（Chrome 未按 clip.scale 输出）",
				wantW, wantH, block.Bounds().Dx(), block.Bounds().Dy(),
			)
		}
		draw.Draw(
			out,
			image.Rect(0, y*scale, width*scale, (y+h)*scale),
			block,
			block.Bounds().Min,
			draw.Src,
		)
	}

	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := encoder.Encode(&buf, out); err != nil {
		return Result{}, fmt.Errorf("编码 PNG: %w", err)
	}

	return Result{
		Slug:   slug,
		PNG:    buf.Bytes(),
		Width:  width * scale,
		Height: totalHeight * scale,
	}, nil
}

// captureSlice 截取文档坐标中的一行。captureBeyondViewport 让视口外的内容也能截到。
func captureSlice(ctx context.Context, width, y, height, scale int) ([]byte, error) {
	action := func(ctx context.Context, t *chromedp.Target) ([]byte, error) {
		res, err := chromedp.Call(ctx, page.CaptureScreenshot, page.CaptureScreenshotParams{
			Format:                page.CaptureScreenshotFormatPng,
			Clip:                  &page.Viewport{X: 0, Y: float64(y), Width: float64(width), Height: float64(height), Scale: float64(scale)},
			CaptureBeyondViewport: boolPtr(true),
			FromSurface:           boolPtr(true),
		})
		if err != nil {
			return nil, err
		}
		return res.Data, nil
	}
	return chromedp.Run(ctx, action)
}

func boolPtr(v bool) *bool { return &v }

// FindChrome 返回 Chrome/Chromium/Edge 的可执行文件路径。
// 优先使用显式路径或 CHROME_PATH，其次按平台常见安装位置查找。
func FindChrome(explicit string) (string, error) {
	candidates := []string{}
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if env := os.Getenv("CHROME_PATH"); env != "" {
		candidates = append(candidates, env)
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		candidates = append(candidates,
			filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(local, "Microsoft", "Edge", "Application", "msedge.exe"),
		)
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if dir := os.Getenv(env); dir != "" {
			candidates = append(candidates,
				filepath.Join(dir, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(dir, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
	}
	candidates = append(candidates,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/microsoft-edge",
	)
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	for _, name := range []string{"google-chrome", "chrome", "chromium", "chromium-browser", "msedge"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("找不到 Chrome/Chromium/Edge，请用环境变量 CHROME_PATH 指定")
}
