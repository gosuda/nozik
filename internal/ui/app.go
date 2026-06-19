package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	musicv1 "github.com/gg582/gozik/api/music/v1"
	playerv1 "github.com/gg582/gozik/api/player/v1"
	"github.com/gg582/nozik/internal/ai"
	"github.com/gg582/nozik/internal/config"
	"github.com/gg582/nozik/internal/curation"
	"github.com/gg582/nozik/internal/db"
	"github.com/gg582/nozik/internal/gzk"
	"github.com/gg582/nozik/internal/model"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	defaultRecLimit = 3
	maxRecLimit     = 50
)

// recCountRe matches phrases like "5개", "3곡", "12 songs", "8 tracks".
var recCountRe = regexp.MustCompile(`(?i)(\d+)\s*(?:개|곡|songs?|tracks?|pieces?|tunes?)`)

// recommendIntent describes what the user wants to do with recommendations.
type recommendIntent int

const (
	intentPlay recommendIntent = iota // play the recommended songs
	intentAdd                         // only add them to the queue
)

// playIntentRe matches explicit play requests. addIntentRe matches explicit
// add-to-queue requests. If neither matches, the default is intentAdd so that
// recommendations do not hijack the current playback.
var (
	playIntentRe = regexp.MustCompile(`(?i)(?:틀어|재생|play|플레이|들려|들어|start)`)
	addIntentRe  = regexp.MustCompile(`(?i)(?:추가해|추가|넣어|넣|add|queue|append|put)`)
)

// parseRecommendIntent extracts whether the user asked to play or just add.
func parseRecommendIntent(text string) recommendIntent {
	// Explicit add takes precedence over generic "recommend" so that queueing
	// requests never unexpectedly start playback.
	if addIntentRe.MatchString(text) {
		return intentAdd
	}
	if playIntentRe.MatchString(text) {
		return intentPlay
	}
	return intentAdd
}

// parseRecommendCount extracts the requested number of recommendations from the
// user's message. It defaults to defaultRecLimit when no count is found.
func parseRecommendCount(text string) int {
	m := recCountRe.FindStringSubmatch(text)
	if m == nil {
		return defaultRecLimit
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return defaultRecLimit
	}
	if n > maxRecLimit {
		return maxRecLimit
	}
	return n
}

// AssetPath resolves a bundled asset file.
func AssetPath(name string) string {
	if dir := os.Getenv("NOZIK_ASSETS_DIR"); dir != "" {
		return filepath.Join(dir, name)
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "assets", name)
	}
	return filepath.Join("assets", name)
}

// App is the top-level GTK application.
type App struct {
	application *gtk.Application
	window      *gtk.ApplicationWindow
	db          *db.DB
	settings    Settings
	gozik       *gzk.Client
	gozikMu     sync.Mutex

	// UI widgets
	chatView       *gtk.TextView
	chatBuffer     *gtk.TextBuffer
	entry          *gtk.Entry
	sendBtn        *gtk.Button
	recBox         *gtk.Box
	actionBox      *gtk.Box
	approveBtn     *gtk.Button
	modifyBtn      *gtk.Button
	denyBtn        *gtk.Button
	songlistBox    *gtk.Grid
	songlistScroll *gtk.ScrolledWindow
	statusLabel    *gtk.Label

	pendingRecs     []model.Recommendation
	recCheckboxes   []*gtk.CheckButton
	pendingIntent   recommendIntent
}

// Settings is the persisted UI configuration.
type Settings struct {
	Provider    model.ProviderKind                       `json:"provider"`
	GeminiKey   string                                   `json:"gemini_key,omitempty"`
	OpenAIKey   string                                   `json:"openai_key,omitempty"`
	GrokKey     string                                   `json:"grok_key,omitempty"`
	KimiKey     string                                   `json:"kimi_key,omitempty"`
	GeminiModel string                                   `json:"gemini_model,omitempty"`
	OpenAIModel string                                   `json:"openai_model,omitempty"`
	GrokModel   string                                   `json:"grok_model,omitempty"`
	KimiModel   string                                   `json:"kimi_model,omitempty"`
	KimiBaseURL string                                   `json:"kimi_base_url,omitempty"`
	ModelsCache map[model.ProviderKind][]model.ModelInfo `json:"models_cache,omitempty"`
	Custom      ai.LuaCustomConfig                       `json:"custom"`
}

// NewApp creates the Nozik GTK app.
func NewApp(database *db.DB) (*App, error) {
	app, err := gtk.ApplicationNew("com.gosuda.nozik.agent", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		return nil, err
	}
	a := &App{
		application: app,
		db:          database,
		settings:    Settings{Provider: model.ProviderGemini},
	}
	_ = a.loadSettings()

	// Connect to an existing Gozik, or start one in the background so the UI
	// appears immediately while Gozik is booting.
	if client, err := gzk.Dial(config.GozikAddr()); err == nil {
		a.gozik = client
		log.Printf("Connected to existing Gozik at %s", config.GozikAddr())
	} else {
		go func() {
			a.gozikMu.Lock()
			defer a.gozikMu.Unlock()
			if a.gozik != nil {
				return
			}
			gozikCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if client, err := gzk.LaunchAndDial(gozikCtx, config.GozikAddr()); err != nil {
				log.Printf("Gozik auto-launch failed: %v", err)
			} else {
				a.gozik = client
				log.Printf("Started Gozik at %s", config.GozikAddr())
			}
		}()
	}

	app.Connect("activate", a.onActivate)
	return a, nil
}

// Run starts the GTK main loop.
func (a *App) Run() int {
	return a.application.Run(os.Args)
}

func (a *App) onActivate() {
	if a.window != nil {
		a.window.Present()
		return
	}
	win, err := gtk.ApplicationWindowNew(a.application)
	if err != nil {
		log.Fatal(err)
	}
	a.window = win
	win.SetTitle("Nozik")
	win.SetDefaultSize(760, 700)
	win.SetSizeRequest(680, 520)
	win.SetPosition(gtk.WIN_POS_CENTER)

	if iconPath := AssetPath(filepath.Join("icons", "nozik.png")); fileExists(iconPath) {
		if pix, err := gdk.PixbufNewFromFile(iconPath); err == nil {
			win.SetIcon(pix)
		}
	}

	a.applyCSS()

	// Header
	hb, _ := gtk.HeaderBarNew()
	hb.SetShowCloseButton(true)
	hb.SetTitle("Nozik")
	hb.SetSubtitle("Ask Nozik. Then Play Music.")
	win.SetTitlebar(hb)

	settingsBtn, _ := gtk.ButtonNewFromIconName("emblem-system-symbolic", gtk.ICON_SIZE_BUTTON)
	settingsBtn.SetTooltipText("Settings")
	settingsBtn.Connect("clicked", func() { a.showSettingsDialog() })
	hb.PackEnd(settingsBtn)

	// Main vertical box
	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 12)
	vbox.SetMarginTop(18)
	vbox.SetMarginBottom(18)
	vbox.SetMarginStart(18)
	vbox.SetMarginEnd(18)
	win.Add(vbox)

	// Chat view
	scroll, _ := gtk.ScrolledWindowNew(nil, nil)
	scroll.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scroll.SetMinContentHeight(320)
	scroll.SetProperty("name", "SonglistScroll")
	a.chatView, _ = gtk.TextViewNew()
	a.chatView.SetEditable(false)
	a.chatView.SetWrapMode(gtk.WRAP_WORD_CHAR)
	a.chatView.SetProperty("name", "LyricsScroll")
	a.chatBuffer, _ = a.chatView.GetBuffer()
	a.appendChat("Nozik", "Hi, which song would you like to play?")
	scroll.Add(a.chatView)
	vbox.PackStart(scroll, true, true, 0)

	// Input row
	inputBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	a.entry, _ = gtk.EntryNew()
	a.entry.SetPlaceholderText("Type a request...")
	a.entry.SetHExpand(true)
	a.entry.Connect("activate", func() { a.onSend() })
	a.sendBtn, _ = gtk.ButtonNewWithLabel("Send")
	a.sendBtn.SetProperty("name", "BtnOpen")
	a.sendBtn.Connect("clicked", func() { a.onSend() })
	inputBox.PackStart(a.entry, true, true, 0)
	inputBox.PackStart(a.sendBtn, false, false, 0)
	vbox.PackStart(inputBox, false, false, 0)

	// Recommendations
	recLabel, _ := gtk.LabelNew("Recommendations")
	recLabel.SetProperty("name", "QueueLabel")
	recLabel.SetHAlign(gtk.ALIGN_START)
	vbox.PackStart(recLabel, false, false, 0)

	a.recBox, _ = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	vbox.PackStart(a.recBox, false, false, 0)

	// Action buttons
	a.actionBox, _ = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	a.actionBox.SetNoShowAll(true)
	a.actionBox.Hide()
	a.approveBtn, _ = gtk.ButtonNewWithLabel("Approve")
	a.approveBtn.SetProperty("name", "BtnOpen")
	a.approveBtn.Connect("clicked", func() { a.onApprove() })
	a.modifyBtn, _ = gtk.ButtonNewWithLabel("Modify")
	a.modifyBtn.SetProperty("name", "btn-ghost")
	a.modifyBtn.Connect("clicked", func() { a.onModify() })
	a.denyBtn, _ = gtk.ButtonNewWithLabel("Deny")
	a.denyBtn.SetProperty("name", "btn-ghost")
	a.denyBtn.Connect("clicked", func() { a.onDeny() })
	a.actionBox.PackStart(a.approveBtn, false, false, 0)
	a.actionBox.PackStart(a.modifyBtn, false, false, 0)
	a.actionBox.PackStart(a.denyBtn, false, false, 0)
	vbox.PackStart(a.actionBox, false, false, 0)

	// Songlist view
	a.songlistScroll, _ = gtk.ScrolledWindowNew(nil, nil)
	a.songlistScroll.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	a.songlistScroll.SetMinContentHeight(140)
	a.songlistScroll.SetProperty("name", "SonglistScroll")
	a.songlistScroll.SetNoShowAll(true)
	a.songlistScroll.Hide()
	a.songlistBox, _ = gtk.GridNew()
	a.songlistBox.SetColumnSpacing(12)
	a.songlistBox.SetRowSpacing(4)
	a.songlistBox.SetMarginStart(8)
	a.songlistBox.SetMarginEnd(8)
	a.songlistBox.SetMarginTop(8)
	a.songlistBox.SetMarginBottom(8)
	a.songlistBox.SetColumnHomogeneous(false)
	a.songlistScroll.Add(a.songlistBox)
	vbox.PackStart(a.songlistScroll, false, false, 0)

	// Status
	a.statusLabel, _ = gtk.LabelNew("")
	a.statusLabel.SetHAlign(gtk.ALIGN_START)
	a.statusLabel.SetProperty("name", "AlbumArtist")
	vbox.PackStart(a.statusLabel, false, false, 0)

	win.ShowAll()
}

func (a *App) applyCSS() {
	provider, _ := gtk.CssProviderNew()
	cssPath := AssetPath(filepath.Join("ui", "nozik.css"))
	if err := provider.LoadFromPath(cssPath); err != nil {
		log.Println("CSS load error:", err)
		return
	}
	screen, _ := gdk.ScreenGetDefault()
	if screen != nil {
		gtk.AddProviderForScreen(screen, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	}
}

func (a *App) appendChat(sender, text string) {
	mark := ""
	switch sender {
	case "Nozik":
		mark = "\nNozik: "
	case "User":
		mark = "\nUser: "
	case "MCP":
		mark = "\nMCP: "
	default:
		mark = "\n" + sender + ": "
	}
	end := a.chatBuffer.GetEndIter()
	a.chatBuffer.Insert(end, mark+text+"\n")
	a.chatView.ScrollToIter(a.chatBuffer.GetEndIter(), 0, false, 0, 0)
}

func (a *App) setStatus(text string) {
	glib.IdleAdd(func() {
		a.statusLabel.SetText(text)
	})
}

func (a *App) onSend() {
	text, _ := a.entry.GetText()
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.entry.SetText("")
	a.appendChat("User", text)

	// Start fresh: clear any previous recommendation / approval state.
	a.clearRecommendationUI()

	emotion := curation.DetectEmotion(text)
	mode := curation.ChooseMode(emotion, strings.Contains(strings.ToLower(text), "new") || strings.Contains(strings.ToLower(text), "discover"))
	limit := parseRecommendCount(text)
	a.pendingIntent = parseRecommendIntent(text)
	ctx := context.Background()

	go func() {
		strategy := a.strategyForSettings()
		if strategy == nil {
			glib.IdleAdd(func() { a.appendChat("Nozik", "No AI provider configured. Please check settings.") })
			return
		}
		req := ai.RecommendationRequest{
			Message:       text,
			Emotion:       emotion,
			DiscoveryMode: mode,
			Limit:         limit,
			SystemContext: fmt.Sprintf("Current emotion: %s. Discovery mode: %s. Use provider 'yt-music' if possible. Do NOT use local taste history; rely only on the user's request.", emotion, mode),
		}
		recs, err := strategy.RecommendTracks(ctx, req)
		if err != nil {
			a.setStatus(fmt.Sprintf("%s error: %v", strategy.Name(), err))
			glib.IdleAdd(func() { a.appendChat("Nozik", "Sorry, I couldn't generate recommendations right now.") })
			return
		}
		filtered := []model.Recommendation{}
		for _, r := range recs {
			if r.Title == "" {
				continue
			}
			filtered = append(filtered, r)
			if len(filtered) >= limit {
				break
			}
		}
		if len(filtered) == 0 {
			glib.IdleAdd(func() { a.appendChat("Nozik", "I couldn't find any recommendations right now.") })
			return
		}
		glib.IdleAdd(func() { a.showRecommendations(filtered) })
	}()
}

func (a *App) strategyForSettings() ai.AIProviderStrategy {
	switch a.settings.Provider {
	case model.ProviderGemini:
		return ai.NewGeminiStrategy(a.settings.GeminiKey, a.settings.GeminiModel)
	case model.ProviderOpenAI:
		return ai.NewOpenAIStrategy(a.settings.OpenAIKey, a.settings.OpenAIModel)
	case model.ProviderGrok:
		return ai.NewGrokStrategy(a.settings.GrokKey, a.settings.GrokModel)
	case model.ProviderKimi:
		return ai.NewKimiStrategy(a.settings.KimiKey, a.settings.KimiModel, a.settings.KimiBaseURL)
	case model.ProviderOther:
		return ai.NewLuaCustomStrategy(a.settings.Custom)
	default:
		return ai.NewGeminiStrategy(a.settings.GeminiKey, a.settings.GeminiModel)
	}
}

// clearRecommendationUI removes the current recommendation buttons, clears the
// internal queue / selection, and hides the approval controls. Call this before
// starting a new request or after Approve / Deny / Modify to flush stale state.
func (a *App) clearRecommendationUI() {
	children := a.recBox.GetChildren()
	for children.Length() > 0 {
		a.recBox.Remove(children.NthData(0).(gtk.IWidget))
		children = a.recBox.GetChildren()
	}
	a.pendingRecs = nil
	a.recCheckboxes = nil
	a.pendingIntent = intentAdd

	songlistChildren := a.songlistBox.GetChildren()
	for songlistChildren.Length() > 0 {
		a.songlistBox.Remove(songlistChildren.NthData(0).(gtk.IWidget))
		songlistChildren = a.songlistBox.GetChildren()
	}
	a.songlistBox.SetColumnHomogeneous(false)

	a.actionBox.Hide()
	a.songlistScroll.Hide()
}

func (a *App) showRecommendations(recs []model.Recommendation) {
	// Clear existing recs.
	children := a.recBox.GetChildren()
	for children.Length() > 0 {
		a.recBox.Remove(children.NthData(0).(gtk.IWidget))
		children = a.recBox.GetChildren()
	}
	a.pendingRecs = recs
	a.recCheckboxes = nil

	a.showRecommendationList(recs)
	a.appendChat("Nozik", "Here are some recommendations. Check the songs you want, then approve.")
}

// showRecommendationList renders recommendations as a table with columns
// for #, Title, Artist and a selection checkbox. All items are checked by default.
//
// Mouse selection follows the usual modifier rules:
//   - plain click -> select only that row
//   - Ctrl/Cmd+click -> toggle that row without affecting other rows
//   - header checkbox -> select / deselect all rows
func (a *App) showRecommendationList(recs []model.Recommendation) {
	grid := a.songlistBox

	// Fixed widths for index/checkbox columns; title/artist columns expand.
	numWidth := 40
	chkWidth := 40

	// Header row.
	headers := []struct {
		txt  string
		name string
	}{
		{"#", "row-head"},
		{"Title", "row-head"},
		{"Artist", "row-head"},
		{"", "row-head"},
	}
	for col, h := range headers {
		lbl, _ := gtk.LabelNew(h.txt)
		lbl.SetProperty("name", h.name)
		lbl.SetHAlign(gtk.ALIGN_START)
		lbl.SetMarginBottom(4)
		grid.Attach(lbl, col, 0, 1, 1)
	}

	selectAllChk, _ := gtk.CheckButtonNew()
	selectAllChk.SetActive(true)
	selectAllChk.SetHAlign(gtk.ALIGN_END)
	selectAllChk.SetTooltipText("Select all")
	selectAllChk.SetSizeRequest(chkWidth, -1)
	grid.Attach(selectAllChk, 3, 0, 1, 1)

	var updateHeaderState func()
	selectAllHandler := selectAllChk.Connect("toggled", func() {
		active := selectAllChk.GetActive()
		for _, c := range a.recCheckboxes {
			c.SetActive(active)
		}
	})

	updateHeaderState = func() {
		active := 0
		for _, c := range a.recCheckboxes {
			if c.GetActive() {
				active++
			}
		}
		selectAllChk.HandlerBlock(selectAllHandler)
		if active == 0 {
			selectAllChk.SetInconsistent(false)
			selectAllChk.SetActive(false)
		} else if active == len(a.recCheckboxes) {
			selectAllChk.SetInconsistent(false)
			selectAllChk.SetActive(true)
		} else {
			selectAllChk.SetInconsistent(true)
		}
		selectAllChk.HandlerUnblock(selectAllHandler)
	}

	for i, r := range recs {
		row := i + 1

		numLbl, _ := gtk.LabelNew(fmt.Sprintf("%d.", i+1))
		numLbl.SetProperty("name", "row-lead")
		numLbl.SetSizeRequest(numWidth, -1)
		numLbl.SetHAlign(gtk.ALIGN_START)
		numLbl.SetTooltipText(r.Reason)

		titleLbl, _ := gtk.LabelNew(r.Title)
		titleLbl.SetProperty("name", "row-title")
		titleLbl.SetHAlign(gtk.ALIGN_START)
		titleLbl.SetEllipsize(3) // PANGO_ELLIPSIZE_END
		titleLbl.SetHExpand(true)
		titleLbl.SetTooltipText(r.Reason)

		artistLbl, _ := gtk.LabelNew(r.Artist)
		artistLbl.SetProperty("name", "row-artist")
		artistLbl.SetHAlign(gtk.ALIGN_START)
		artistLbl.SetEllipsize(3)
		artistLbl.SetHExpand(true)
		artistLbl.SetTooltipText(r.Reason)

		chk, _ := gtk.CheckButtonNew()
		chk.SetActive(true)
		chk.SetHAlign(gtk.ALIGN_END)
		chk.SetTooltipText("Include this song")
		chk.SetSizeRequest(chkWidth, -1)

		grid.Attach(numLbl, 0, row, 1, 1)
		grid.Attach(titleLbl, 1, row, 1, 1)
		grid.Attach(artistLbl, 2, row, 1, 1)
		grid.Attach(chk, 3, row, 1, 1)

		a.recCheckboxes = append(a.recCheckboxes, chk)

		chk.Connect("toggled", updateHeaderState)

		// Row selection with modifier-aware behavior.
		idx := i
		rowClickFn := func(event *gdk.Event) bool {
			ev := gdk.EventButtonNewFromEvent(event)
			if ev == nil {
				return false
			}
			if isMultiSelectModifier(ev.State()) {
				// Ctrl/Cmd+click toggles just this row.
				active := chk.GetActive()
				chk.SetActive(!active)
			} else {
				// Plain click selects only this row.
				for j, c := range a.recCheckboxes {
					c.SetActive(j == idx)
				}
			}
			return true
		}
		numLbl.Connect("button-press-event", rowClickFn)
		titleLbl.Connect("button-press-event", rowClickFn)
		artistLbl.Connect("button-press-event", rowClickFn)
	}

	grid.ShowAll()
	a.songlistScroll.SetNoShowAll(false)
	a.songlistScroll.ShowAll()

	a.approveBtn.SetLabel("Approve Selected")
	a.actionBox.SetNoShowAll(false)
	a.actionBox.ShowAll()
}

// isMultiSelectModifier reports whether a button-press event state indicates
// that the user wants additive selection (Ctrl on Linux/Windows, Cmd/Super on macOS).
func isMultiSelectModifier(state uint) bool {
	mods := uint(gdk.CONTROL_MASK) |
		uint(gdk.MOD2_MASK) |
		uint(gdk.SUPER_MASK) |
		uint(gdk.META_MASK)
	return state&mods != 0
}


func (a *App) onApprove() {
	var targets []model.Recommendation
	for i, chk := range a.recCheckboxes {
		if i >= len(a.pendingRecs) {
			break
		}
		if chk.GetActive() {
			targets = append(targets, a.pendingRecs[i])
		}
	}
	if len(targets) == 0 {
		a.appendChat("Nozik", "No songs selected. Check at least one song and try again.")
		return
	}

	intent := a.pendingIntent

	// Flush the recommendation queue and approval UI as soon as the user acts.
	a.clearRecommendationUI()

	if len(targets) == 1 {
		a.appendChat("Nozik", fmt.Sprintf("Approved '%s' by %s.", targets[0].Title, targets[0].Artist))
	} else {
		a.appendChat("Nozik", fmt.Sprintf("Approved %d songs.", len(targets)))
	}
	a.appendChat("MCP", "Searching an Installation...")

	go func() {
		ctx := context.Background()
		a.gozikMu.Lock()
		if a.gozik == nil {
			gozikCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			client, err := gzk.LaunchAndDial(gozikCtx, config.GozikAddr())
			if err != nil {
				a.gozikMu.Unlock()
				a.setStatus("Gozik not reachable and could not be started: " + err.Error())
				glib.IdleAdd(func() {
					a.appendChat("Nozik", "Could not connect to or start Gozik. Make sure gozik binary is available or set NOZIK_GOZIK_BIN.")
				})
				return
			}
			a.gozik = client
		}
		a.gozikMu.Unlock()

		glib.IdleAdd(func() { a.appendChat("Nozik", "Approved. Calling a Gozik Player...") })

		// Verify we are talking to the Gozik PlayerService and discover providers.
		// Gozik polls for providers in the background, so briefly retry if the list is empty.
		var providers []*playerv1.ProviderInfo
		var err error
		for i := 0; i < 10; i++ {
			providers, err = a.gozik.ListProviders(ctx)
			if err != nil {
				a.setStatus("Gozik PlayerService handshake failed: " + err.Error())
				glib.IdleAdd(func() {
					a.appendChat("Nozik", "Could not talk to Gozik PlayerService. Port 50051 is reserved for Gozik; providers now default to 50052.")
				})
				return
			}
			if len(providers) > 0 {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if len(providers) == 0 {
			a.setStatus("No music providers discovered by Gozik")
			glib.IdleAdd(func() {
				a.appendChat("Nozik", "Gozik is running but no music provider was found. Start gozik-yt-music or gozik-spotify first, and make sure Gozik was started before the provider.")
			})
			return
		}

		// Remember where the new songs will start in the queue.
		var startIndex int32
		if q, err := a.gozik.GetQueue(ctx); err == nil && q != nil {
			startIndex = int32(len(q.GetTracks()))
		}

		added := 0
		failed := []string{}
		for _, rec := range targets {
			providerID := rec.ProviderID
			if providerID == "" || providerID == "yt-music" {
				providerID = providers[0].GetId()
			}

			if err := a.addTrackWithFallbacks(ctx, providerID, rec); err != nil {
				log.Printf("Failed to add track %q: %v", rec.Title, err)
				failed = append(failed, rec.Title)
				continue
			}
			added++
		}

		// Only start playback when the user explicitly asked to play, and start
		// from the first newly added track instead of jumping back to the top.
		if intent == intentPlay && added > 0 {
			if err := a.gozik.PlayIndex(ctx, startIndex); err != nil {
				log.Printf("PlayIndex error: %v", err)
				// Fall back to plain Play if the player doesn't support index selection.
				_ = a.gozik.Play(ctx)
			}
		}

		if added == 0 {
			glib.IdleAdd(func() { a.appendChat("Nozik", "Sorry, none of the approved songs could be played right now.") })
		} else if intent == intentPlay {
			if len(failed) > 0 {
				glib.IdleAdd(func() {
					a.appendChat("Nozik", fmt.Sprintf("Playing from the new songs. Could not resolve: %s.", strings.Join(failed, ", ")))
				})
			} else {
				glib.IdleAdd(func() { a.appendChat("Nozik", "Enjoy the music! I will wait for your next request.") })
			}
		} else {
			if len(failed) > 0 {
				glib.IdleAdd(func() {
					a.appendChat("Nozik", fmt.Sprintf("Added %d song(s) to the queue. Could not resolve: %s.", added, strings.Join(failed, ", ")))
				})
			} else {
				glib.IdleAdd(func() { a.appendChat("Nozik", fmt.Sprintf("Added %d song(s) to the queue.", added)) })
			}
		}
	}()
}

// addTrackWithFallbacks tries multiple strategies to queue a recommendation:
//  1. Use a stored provider_track_id if it still resolves.
//  2. Search title + artist and try the best playable match.
//  3. If that fails, retry the search with just the title, then just the artist.
//  4. Finally try the original provider_track_id (if any) with the default provider.
//
// This prevents one failed resolve from aborting a batch and gives flaky
// providers / network a few chances to succeed.
func (a *App) addTrackWithFallbacks(ctx context.Context, providerID string, rec model.Recommendation) error {
	if rec.ProviderTrackID != "" {
		if err := a.gozik.ResolveProviderStream(ctx, providerID, rec.ProviderTrackID); err == nil {
			return a.gozik.AddProviderTrack(ctx, providerID, rec.ProviderTrackID)
		}
	}

	queries := []string{
		rec.Title + " " + rec.Artist,
		rec.Title,
	}
	if rec.Artist != "" {
		queries = append(queries, rec.Artist+" "+rec.Title)
	}

	for _, q := range queries {
		if err := a.searchAndAddPlayable(ctx, providerID, rec, q); err == nil {
			return nil
		}
	}

	return fmt.Errorf("could not resolve track %q on Gozik", rec.Title)
}

// searchAndAddPlayable searches for rec through Gozik, scores the results like
// a human would (preferring exact title/artist matches and penalising fan-made,
// cover, live, remix, loop, etc.), then queues the best playable candidate.
func (a *App) searchAndAddPlayable(ctx context.Context, providerID string, rec model.Recommendation, query string) error {
	tracks, err := a.gozik.SearchTracks(ctx, providerID, query, 8)
	if err != nil {
		return fmt.Errorf("SearchTracks error: %w", err)
	}
	if len(tracks) == 0 {
		return fmt.Errorf("no search results")
	}

	scored := make([]scoredTrack, 0, len(tracks))
	for _, t := range tracks {
		scored = append(scored, scoredTrack{
			track: t,
			score: scoreTrack(&rec, t),
		})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

	for _, st := range scored {
		if err := a.gozik.ResolveProviderStream(ctx, providerID, st.track.GetId()); err != nil {
			continue
		}
		if err := a.gozik.AddProviderTrack(ctx, providerID, st.track.GetId()); err != nil {
			return fmt.Errorf("AddProviderTrack error: %w", err)
		}
		return nil
	}
	return fmt.Errorf("no playable track found")
}

type scoredTrack struct {
	track *musicv1.Track
	score float64
}

// scoreTrack approximates how a human would rank a Gozik search result:
// exact title/artist matches score highest; fan-made, cover, live, remix,
// loop and extremely long tracks are penalised.
func scoreTrack(rec *model.Recommendation, t *musicv1.Track) float64 {
	score := 0.0
	recTitle := normalize(rec.Title)
	recArtist := normalize(rec.Artist)
	title := normalize(t.GetTitle())

	// Title matching
	if title == recTitle {
		score += 100
	} else if strings.Contains(title, recTitle) {
		score += 60
	} else {
		score += wordOverlapScore(recTitle, title) * 35
	}

	// Artist matching
	artistNames := make([]string, 0, len(t.GetArtists()))
	for _, a := range t.GetArtists() {
		artistNames = append(artistNames, normalize(a.GetName()))
	}
	artistStr := strings.Join(artistNames, " ")
	if recArtist != "" {
		if strings.Contains(artistStr, recArtist) {
			score += 50
		} else {
			score += wordOverlapScore(recArtist, artistStr) * 25
		}
	}

	titleLower := strings.ToLower(t.GetTitle())

	// Penalise fan-made / cover / live / remix / loop / long compilations.
	blacklist := []string{
		"fan made", "fanmade", "cover", "covers", "live", "remix", "bootleg",
		"instrumental", "karaoke", "8d", "reverb", "slowed", "sped up",
		"nightcore", "extended", "loop", "1 hour", "10 hours", "lyrics",
		"traducida", "sub es", "subtitulado", "audio", "mv", "full album",
	}
	for _, kw := range blacklist {
		if strings.Contains(titleLower, kw) {
			score -= 35
		}
	}

	// Reward official-ish channels (e.g. "- Topic", "VEVO", "Official").
	officialKws := []string{"topic", "vevo", "official"}
	for _, name := range artistNames {
		nameLower := strings.ToLower(name)
		for _, kw := range officialKws {
			if strings.Contains(nameLower, kw) {
				score += 25
			}
		}
	}

	// Duration penalties for extremely long tracks (likely loops/compilations).
	if t.GetDurationMs() > 10*60*1000 {
		score -= 20
	}
	if t.GetDurationMs() > 60*60*1000 {
		score -= 60
	}

	return score
}

// normalize lowercases a string and keeps only letters, digits and spaces.
func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "&", "and")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// wordOverlapScore returns the fraction of words in a that also appear in b.
func wordOverlapScore(a, b string) float64 {
	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)
	if len(wordsA) == 0 {
		return 0
	}
	setB := make(map[string]struct{}, len(wordsB))
	for _, w := range wordsB {
		setB[w] = struct{}{}
	}
	matches := 0
	for _, w := range wordsA {
		if _, ok := setB[w]; ok {
			matches++
		}
	}
	return float64(matches) / float64(len(wordsA))
}

func (a *App) onModify() {
	a.clearRecommendationUI()
	a.appendChat("Nozik", "Modify mode: please refine your request and send again.")
}

func (a *App) onDeny() {
	a.clearRecommendationUI()
	a.appendChat("Nozik", "Got it. Let me know if you want something else.")
}

func (a *App) loadSettings() error {
	p := config.SettingsPath()
	if _, err := os.Stat(p); err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &a.settings)
}

func (a *App) saveSettings() error {
	if err := config.EnsureAppDir(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(a.settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(config.SettingsPath(), b, 0644)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
