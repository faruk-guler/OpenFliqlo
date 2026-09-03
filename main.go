package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// --- Configuration & Constants ---
const (
	AppVersion        = "1.2.0.2"
	DesignDPI         = 72
	DesignHeight      = 1080.0
	DesignWidth       = 1920.0
	MouseSensitivity  = 150.0 // Base exit threshold distance squared at 1080p
	StartupGraceTicks = 15    // ~500ms at 30 TPS before mouse check activates
)

// Config holds user-configurable options loaded from config.json.
type Config struct {
	Language  string `json:"language"` // "auto", "tr", "en"
	URLText   string `json:"url"`
	ShowURL   bool   `json:"show_url"`
	ShowYear  bool   `json:"show_year"`
	Format24h bool   `json:"format_24h"`
}

var currentConfig = Config{
	Language:  "auto",
	URLText:   "www.farukguler.com",
	ShowURL:   true,
	ShowYear:  true,
	Format24h: true,
}

var activeConfigPath string

// --- Design Tokens (Modern Aesthetics) ---
var (
	BackgroundColor  = color.RGBA{5, 5, 5, 255}        // Deep black
	PrimaryTextColor = color.White                     // High-contrast white
	SubtleTextColor  = color.RGBA{160, 160, 160, 180} // Low-contrast URL
)

// --- Internationalization (Turkish & English) ---
var (
	turkishMonths = []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}
	turkishDays   = []string{"Pazar", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi"}

	englishMonths = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	englishDays   = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
)

// resolveLanguage returns "tr" or "en" based on user configuration or Windows system locale.
func resolveLanguage() string {
	l := strings.ToLower(strings.TrimSpace(currentConfig.Language))
	if l == "tr" || l == "turkish" {
		return "tr"
	}
	if l == "en" || l == "english" {
		return "en"
	}

	// Auto detect from Windows system locale
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetUserDefaultLocaleName")
	buf := make([]uint16, 85)
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r > 0 {
		locale := syscall.UTF16ToString(buf)
		if strings.HasPrefix(strings.ToLower(locale), "tr") {
			return "tr"
		}
	}
	return "en"
}

// --- Assets ---
//go:embed font.ttf
var fontData []byte

var parsedFont *opentype.Font

func getParsedFont() *opentype.Font {
	if parsedFont == nil {
		tt, err := opentype.Parse(fontData)
		if err != nil {
			log.Fatalf("Fatal: Failed to parse embedded font: %v", err)
		}
		parsedFont = tt
	}
	return parsedFont
}

// getConfigPaths returns prioritized candidate locations for config.json.
func getConfigPaths() []string {
	var paths []string

	// 1. Same directory as the executable (portable / standalone)
	if exePath, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exePath), "config.json"))
	}

	// 2. Working directory
	paths = append(paths, "config.json")

	// 3. %APPDATA%\OpenFliqlo\config.json (user configuration when installed in System32 via GPO)
	if appData := os.Getenv("APPDATA"); appData != "" {
		paths = append(paths, filepath.Join(appData, "OpenFliqlo", "config.json"))
	}

	return paths
}

// getWritableConfigPath returns the most appropriate file path for saving/creating config.json.
func getWritableConfigPath() string {
	// If an existing config was loaded, prefer editing it
	if activeConfigPath != "" {
		return activeConfigPath
	}

	// Check if executable directory is writable
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		p := filepath.Join(exeDir, "config.json")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		testFile := filepath.Join(exeDir, ".write_test")
		if err := os.WriteFile(testFile, []byte(""), 0666); err == nil {
			_ = os.Remove(testFile)
			return p
		}
	}

	// Fallback to %APPDATA%\OpenFliqlo\config.json (always user-writable)
	if appData := os.Getenv("APPDATA"); appData != "" {
		dir := filepath.Join(appData, "OpenFliqlo")
		_ = os.MkdirAll(dir, 0755)
		return filepath.Join(dir, "config.json")
	}

	return "config.json"
}

// loadConfig reads config.json if present, preserving defaults for omitted fields.
func loadConfig() {
	paths := getConfigPaths()
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			// Pre-fill cfg with currentConfig to prevent zero-value overwrite of omitted booleans
			cfg := currentConfig
			if err := json.Unmarshal(data, &cfg); err == nil {
				currentConfig = cfg
				activeConfigPath = p
				return
			}
		}
	}
}

// --- Game Engine Logic ---
type Game struct {
	lastMouseX   int
	lastMouseY   int
	startupTicks int
	screenWidth  int
	screenHeight int

	clockFont font.Face
	dateFont  font.Face
	urlFont   font.Face

	// Layout and render cache (prevents 60 allocations/sec)
	cachedMinute  int
	cachedTimeStr string
	cachedDateStr string
	clockX        int
	clockY        int
	dateX         int
	dateY         int
	urlX          int
	urlY          int
	cacheValid    bool
}

// getScale calculates resolution scale, safe for any aspect ratio (portrait, square, ultrawide).
func (g *Game) getScale() float64 {
	scaleH := float64(g.screenHeight) / DesignHeight
	scaleW := float64(g.screenWidth) / DesignWidth
	if scaleW < scaleH {
		return scaleW
	}
	return scaleH
}

// updateFontsAndLayout scales font sizes and marks layout dirty.
func (g *Game) updateFontsAndLayout() {
	scale := g.getScale()

	tt := getParsedFont()
	g.clockFont = g.mustCreateFace(tt, 450*scale)
	g.dateFont = g.mustCreateFace(tt, 90*scale)
	g.urlFont = g.mustCreateFace(tt, 35*scale)

	g.cacheValid = false
}

func (g *Game) mustCreateFace(tt *opentype.Font, size float64) font.Face {
	face, err := opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    size,
		DPI:     DesignDPI,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatalf("Fatal: Failed to create font face: %v", err)
	}
	return face
}

// recalculateLayout computes string contents, dimensions, and positions.
func (g *Game) recalculateLayout(now time.Time) {
	scale := float32(g.getScale())

	// Format time
	if currentConfig.Format24h {
		g.cachedTimeStr = now.Format("15:04")
	} else {
		g.cachedTimeStr = now.Format("03:04")
	}

	// Format date based on resolved language
	lang := resolveLanguage()
	if lang == "en" {
		if currentConfig.ShowYear {
			// English: "Thursday, September 3, 2026"
			g.cachedDateStr = fmt.Sprintf("%s, %s %d, %d",
				englishDays[now.Weekday()],
				englishMonths[now.Month()-1],
				now.Day(),
				now.Year())
		} else {
			// English: "Thursday, September 3"
			g.cachedDateStr = fmt.Sprintf("%s, %s %d",
				englishDays[now.Weekday()],
				englishMonths[now.Month()-1],
				now.Day())
		}
	} else {
		if currentConfig.ShowYear {
			// Turkish: "3 Eylül 2026, Perşembe"
			g.cachedDateStr = fmt.Sprintf("%d %s %d, %s",
				now.Day(),
				turkishMonths[now.Month()-1],
				now.Year(),
				turkishDays[now.Weekday()])
		} else {
			// Turkish: "3 Eylül, Perşembe"
			g.cachedDateStr = fmt.Sprintf("%d %s, %s",
				now.Day(),
				turkishMonths[now.Month()-1],
				turkishDays[now.Weekday()])
		}
	}

	// 1. Measure clock
	bClock := text.BoundString(g.clockFont, g.cachedTimeStr)
	clockW := float32(bClock.Dx())
	clockH := float32(bClock.Dy())

	// 2. Measure date
	bDate := text.BoundString(g.dateFont, g.cachedDateStr)
	dateW := float32(bDate.Dx())
	dateH := float32(bDate.Dy())

	// 3. Spacing between clock and date
	gap := 50 * scale

	// Total block visual height
	totalContentH := clockH + gap + dateH

	// Vertical centering for the entire combined block
	blockVisualTop := (float32(g.screenHeight) - totalContentH) / 2

	// Baseline calculations:
	// text.Draw takes baseline Y. Visual top of glyphs is baselineY + b.Min.Y (where Min.Y < 0).
	// Therefore: baselineY = visualTop - b.Min.Y.
	clockBaselineY := blockVisualTop - float32(bClock.Min.Y)
	g.clockX = int((float32(g.screenWidth)-clockW)/2 - float32(bClock.Min.X))
	g.clockY = int(clockBaselineY)

	dateVisualTop := blockVisualTop + clockH + gap
	dateBaselineY := dateVisualTop - float32(bDate.Min.Y)
	g.dateX = int((float32(g.screenWidth)-dateW)/2 - float32(bDate.Min.X))
	g.dateY = int(dateBaselineY)

	// URL positioning in top-right corner
	if currentConfig.ShowURL && currentConfig.URLText != "" {
		bURL := text.BoundString(g.urlFont, currentConfig.URLText)
		g.urlX = int(float32(g.screenWidth) - float32(bURL.Dx()) - (50 * scale))
		g.urlY = int(70 * scale)
	}

	g.cachedMinute = now.Minute()
	g.cacheValid = true
}

// Update handles input detection and exit conditions.
func (g *Game) Update() error {
	mx, my := ebiten.CursorPosition()

	// Grace period on launch to prevent accidental immediate exit
	if g.startupTicks < StartupGraceTicks {
		g.startupTicks++
		g.lastMouseX, g.lastMouseY = mx, my
		return nil
	}

	if !ebiten.IsFullscreen() {
		ebiten.SetFullscreen(true)
	}

	if g.isInputDetected(mx, my) {
		os.Exit(0)
	}

	return nil
}

// isInputDetected checks for mouse movement, clicks, or keyboard activity.
func (g *Game) isInputDetected(mx, my int) bool {
	// 1. Mouse Movement (scaled with resolution so high-DPI displays feel natural)
	dx, dy := mx-g.lastMouseX, my-g.lastMouseY
	scale := g.getScale()
	threshold := MouseSensitivity * scale * scale
	if float64(dx*dx+dy*dy) > threshold {
		return true
	}

	// 2. Mouse Clicks
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		return true
	}

	// 3. Keyboard Input
	if len(ebiten.AppendInputChars(nil)) > 0 {
		return true
	}
	for k := ebiten.Key(0); k <= ebiten.KeyMax; k++ {
		if ebiten.IsKeyPressed(k) {
			return true
		}
	}

	return false
}

// Draw renders the visual elements using cached values.
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(BackgroundColor)

	now := time.Now()
	if !g.cacheValid || now.Minute() != g.cachedMinute {
		g.recalculateLayout(now)
	}

	// Render URL
	if currentConfig.ShowURL && currentConfig.URLText != "" {
		text.Draw(screen, currentConfig.URLText, g.urlFont, g.urlX, g.urlY, SubtleTextColor)
	}

	// Render Clock and Date
	text.Draw(screen, g.cachedTimeStr, g.clockFont, g.clockX, g.clockY, PrimaryTextColor)
	text.Draw(screen, g.cachedDateStr, g.dateFont, g.dateX, g.dateY, PrimaryTextColor)
}

// Layout handles window size changes dynamically.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	if outsideWidth <= 0 || outsideHeight <= 0 {
		return 1920, 1080
	}
	if outsideWidth != g.screenWidth || outsideHeight != g.screenHeight {
		g.screenWidth = outsideWidth
		g.screenHeight = outsideHeight
		g.updateFontsAndLayout()
	}
	return g.screenWidth, g.screenHeight
}

func main() {
	loadConfig()
	handleWindowsArguments()

	game := &Game{}

	// Screensaver için tam ekran, çerçevesiz ve her zaman en üstte (topmost) pencere ayarları
	ebiten.SetWindowTitle("OpenFliqlo")
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	// Aktif monitörün gerçek fiziksel çözünürlüğünü al
	if m := ebiten.Monitor(); m != nil {
		w, h := m.Size()
		if w > 0 && h > 0 {
			game.screenWidth, game.screenHeight = w, h
			ebiten.SetWindowSize(w, h)
		}
	}
	if game.screenWidth == 0 || game.screenHeight == 0 {
		w, h := ebiten.ScreenSizeInFullscreen()
		if w > 0 && h > 0 {
			game.screenWidth, game.screenHeight = w, h
			ebiten.SetWindowSize(w, h)
		} else {
			game.screenWidth, game.screenHeight = 1920, 1080
		}
	}
	game.updateFontsAndLayout()

	ebiten.SetFullscreen(true)
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	ebiten.SetTPS(30) // Düşük CPU & GPU kullanımı, akıcı girdi hassasiyeti

	if err := ebiten.RunGame(game); err != nil {
		log.Fatalf("Fatal: Application error: %v", err)
	}
}

// handleWindowsArguments handles standard screensaver command-line flags.
func handleWindowsArguments() {
	if len(os.Args) > 1 {
		arg := strings.ToLower(os.Args[1])

		// Configure flag (/c or /c:<HWND> or -c)
		if strings.HasPrefix(arg, "/c") || strings.HasPrefix(arg, "-c") {
			var parentHWND uintptr
			if idx := strings.Index(arg, ":"); idx != -1 {
				if val, err := strconv.ParseUint(strings.TrimSpace(arg[idx+1:]), 10, 64); err == nil {
					parentHWND = uintptr(val)
				}
			} else if len(os.Args) > 2 {
				if val, err := strconv.ParseUint(strings.TrimSpace(os.Args[2]), 10, 64); err == nil {
					parentHWND = uintptr(val)
				}
			}

			showWindowsConfigDialog(parentHWND)
			os.Exit(0)
		}

		// Preview flag (/p or /p:<HWND> or -p)
		if strings.HasPrefix(arg, "/p") || strings.HasPrefix(arg, "-p") {
			os.Exit(0)
		}
	}
}

// showWindowsConfigDialog displays a native Windows message box for screensaver configuration.
func showWindowsConfigDialog(parentHWND uintptr) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBoxW := user32.NewProc("MessageBoxW")

	lang := resolveLanguage()

	cfgFile := activeConfigPath
	if cfgFile == "" {
		cfgFile = getWritableConfigPath()
	}

	var title, message string
	if lang == "en" {
		title = "OpenFliqlo Settings"
		timeFmt := "24-Hour (15:04)"
		if !currentConfig.Format24h {
			timeFmt = "12-Hour (03:04)"
		}
		yearFmt := "Enabled"
		if !currentConfig.ShowYear {
			yearFmt = "Disabled"
		}
		urlDisplay := "Enabled"
		if !currentConfig.ShowURL {
			urlDisplay = "Disabled"
		}
		message = fmt.Sprintf(
			"OpenFliqlo - Minimalist Screensaver v%s\n\n"+
				"• Language: English\n"+
				"• Time Format: %s\n"+
				"• Year Display: %s\n"+
				"• URL Display: %s\n"+
				"• URL Text: %s\n\n"+
				"Config File:\n%s\n\n"+
				"Would you like to open the configuration file in Notepad to edit settings?",
			AppVersion,
			timeFmt,
			yearFmt,
			urlDisplay,
			currentConfig.URLText,
			cfgFile,
		)
	} else {
		title = "OpenFliqlo Ayarları"
		timeFmt := "24 Saat (15:04)"
		if !currentConfig.Format24h {
			timeFmt = "12 Saat (03:04)"
		}
		yearFmt := "Açık"
		if !currentConfig.ShowYear {
			yearFmt = "Kapalı"
		}
		urlDisplay := "Açık"
		if !currentConfig.ShowURL {
			urlDisplay = "Kapalı"
		}
		message = fmt.Sprintf(
			"OpenFliqlo - Minimalist Ekran Koruyucu v%s\n\n"+
				"• Dil: Türkçe\n"+
				"• Saat Formatı: %s\n"+
				"• Yıl Gösterimi: %s\n"+
				"• URL Gösterimi: %s\n"+
				"• URL Metni: %s\n\n"+
				"Yapılandırma Dosyası:\n%s\n\n"+
				"Ayarları düzenlemek için yapılandırma dosyasını Not Defteri ile açmak ister misiniz?",
			AppVersion,
			timeFmt,
			yearFmt,
			urlDisplay,
			currentConfig.URLText,
			cfgFile,
		)
	}

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	msgPtr, _ := syscall.UTF16PtrFromString(message)

	// MB_YESNO | MB_ICONINFORMATION = 0x00000004 | 0x00000040 = 0x44
	ret, _, _ := messageBoxW.Call(parentHWND, uintptr(unsafe.Pointer(msgPtr)), uintptr(unsafe.Pointer(titlePtr)), 0x44)

	// IDYES = 6: If user clicked Yes, ensure file exists and open it with notepad.exe
	if ret == 6 {
		if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
			dir := filepath.Dir(cfgFile)
			_ = os.MkdirAll(dir, 0755)
			data, _ := json.MarshalIndent(currentConfig, "", "  ")
			_ = os.WriteFile(cfgFile, data, 0666)
		}
		_ = exec.Command("notepad.exe", cfgFile).Start()
	}
}
