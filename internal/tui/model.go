package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/obey-installer/internal/app"
	"github.com/Obedience-Corp/obey-installer/internal/source"
	"github.com/Obedience-Corp/obey-installer/internal/tui/anim"
	"github.com/Obedience-Corp/obey-installer/internal/tui/components"
	"github.com/Obedience-Corp/obey-installer/internal/tui/theme"
)

type screen int

const (
	screenBoot screen = iota
	screenHome
	screenInstall
	screenUpdate
	screenList
	screenBrowse
	screenUninstall
	screenMarketplace
	screenDoctor
	screenShell
	screenHelp
	screenProgress
	screenResult
	screenConfirm
)

type tickMsg time.Time

type statusMsg struct {
	sum app.StatusSummary
	err error
}

type listMsg struct {
	res app.ListResult
	err error
}

type browseMsg struct {
	res app.BrowseResult
	err error
}

type doctorMsg struct {
	checks []app.DoctorCheck
}

type marketMsg struct {
	views []source.ListView
	err   error
}

type opDoneMsg struct {
	title   string
	body    string
	err     error
	success bool
}

type progressMsg struct {
	ev app.ProgressEvent
}

type model struct {
	opts      Options
	styles    theme.Styles
	reduced   bool
	width     int
	height    int
	screen    screen
	prev      screen
	cursor    int
	frame     int
	bootLeft  int // frames remaining on splash
	status    app.StatusSummary
	statusErr error
	err       error
	quitErr   error
	help      bool

	// install
	channelIdx int
	channels   []string

	// list / browse / uninstall
	list       app.ListResult
	browse     app.BrowseResult
	browseFlat []app.BrowseEntry
	productF   string
	kindF      string

	// marketplace
	markets    []source.ListView
	marketMode string // list | add
	addInput   textinput.Model

	// doctor
	checks []app.DoctorCheck

	// shell
	shellSnippet string
	shellOnPath  bool
	shellBin     string

	// progress / result
	progress    app.ProgressEvent
	resultTitle string
	resultBody  string
	resultOK    bool

	// confirm
	confirmMsg string
	confirmYes bool
	confirmAct string // uninstall
	confirmArg string

	// op in flight cancel
	opCancel context.CancelFunc
	busy     bool
}

var homeItems = []string{
	"Install Festival suite",
	"Update Festival",
	"Installed packages",
	"Browse catalog",
	"Uninstall package",
	"Marketplaces",
	"Doctor",
	"Shell / PATH setup",
	"Quit",
}

func newModel(opts Options) model {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	ti := textinput.New()
	ti.Placeholder = "https://github.com/org/marketplace.git"
	ti.CharLimit = 256
	ti.Width = 48
	m := model{
		opts:       opts,
		styles:     theme.New(),
		reduced:    theme.ReducedMotion(),
		screen:     screenBoot,
		bootLeft:   12, // ~1.2s at 100ms
		channels:   []string{"stable", "rc", "dev"},
		channelIdx: 0,
		addInput:   ti,
		width:      80,
		height:     24,
	}
	if m.reduced {
		m.bootLeft = 1
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), loadStatus())
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func loadStatus() tea.Cmd {
	return func() tea.Msg {
		sum, err := app.Status(context.Background())
		return statusMsg{sum: sum, err: err}
	}
}

func loadList() tea.Cmd {
	return func() tea.Msg {
		res, err := app.ListInstalled(context.Background())
		return listMsg{res: res, err: err}
	}
}

func loadBrowse(product, kind string) tea.Cmd {
	return func() tea.Msg {
		res, err := app.Browse(context.Background(), app.BrowseOptions{Product: product, Kind: kind})
		return browseMsg{res: res, err: err}
	}
}

func loadDoctor() tea.Cmd {
	return func() tea.Msg {
		return doctorMsg{checks: app.Doctor(context.Background())}
	}
}

func loadMarkets() tea.Cmd {
	return func() tea.Msg {
		views, err := app.MarketplaceList(context.Background())
		return marketMsg{views: views, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.frame++
		if m.screen == screenBoot {
			m.bootLeft--
			if m.bootLeft <= 0 {
				m.screen = screenHome
			}
		}
		return m, tickCmd()

	case statusMsg:
		m.status = msg.sum
		m.statusErr = msg.err
		return m, nil

	case listMsg:
		m.list = msg.res
		m.err = msg.err
		m.cursor = 0
		return m, nil

	case browseMsg:
		m.browse = msg.res
		m.err = msg.err
		m.browseFlat = flattenBrowse(msg.res)
		m.cursor = 0
		return m, nil

	case doctorMsg:
		m.checks = msg.checks
		return m, nil

	case marketMsg:
		m.markets = msg.views
		m.err = msg.err
		m.cursor = 0
		return m, nil

	case progressMsg:
		m.progress = msg.ev
		return m, nil

	case opDoneMsg:
		m.busy = false
		m.opCancel = nil
		m.screen = screenResult
		m.resultTitle = msg.title
		m.resultBody = msg.body
		m.resultOK = msg.success
		m.err = msg.err
		return m, loadStatus()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.help && msg.String() != "?" && msg.String() != "esc" {
		m.help = false
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		if m.busy && m.opCancel != nil {
			m.opCancel()
			m.busy = false
			m.screen = screenHome
			return m, nil
		}
		return m, tea.Quit
	case "q":
		if m.screen == screenHome || m.screen == screenBoot {
			return m, tea.Quit
		}
		if m.screen == screenMarketplace && m.marketMode == "add" {
			m.marketMode = "list"
			return m, nil
		}
		m.screen = screenHome
		m.cursor = 0
		m.err = nil
		return m, nil
	case "?":
		m.help = !m.help
		return m, nil
	case "esc":
		if m.help {
			m.help = false
			return m, nil
		}
		if m.screen == screenBoot {
			m.screen = screenHome
			return m, nil
		}
		if m.screen == screenMarketplace && m.marketMode == "add" {
			m.marketMode = "list"
			return m, nil
		}
		if m.screen == screenHome {
			return m, tea.Quit
		}
		m.screen = screenHome
		m.cursor = 0
		m.err = nil
		return m, nil
	case "enter", " ":
		if m.screen == screenBoot {
			m.screen = screenHome
			return m, nil
		}
		return m.handleEnter()
	}

	// Marketplace add text input
	if m.screen == screenMarketplace && m.marketMode == "add" {
		var cmd tea.Cmd
		m.addInput, cmd = m.addInput.Update(msg)
		if msg.String() == "enter" {
			return m.submitMarketplaceAdd()
		}
		return m, cmd
	}

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		max := m.maxCursor()
		if m.cursor < max {
			m.cursor++
		}
	case "left", "h":
		if m.screen == screenInstall && m.channelIdx > 0 {
			m.channelIdx--
		}
		if m.screen == screenConfirm {
			m.confirmYes = true
		}
	case "right", "l":
		if m.screen == screenInstall && m.channelIdx < len(m.channels)-1 {
			m.channelIdx++
		}
		if m.screen == screenConfirm {
			m.confirmYes = false
		}
	case "y":
		if m.screen == screenConfirm {
			m.confirmYes = true
			return m.handleEnter()
		}
	case "n":
		if m.screen == screenConfirm {
			m.confirmYes = false
			return m.handleEnter()
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if m.screen == screenHome {
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(homeItems) {
				m.cursor = idx
				return m.handleEnter()
			}
		}
	case "r":
		if m.screen == screenMarketplace {
			return m, loadMarkets()
		}
		if m.screen == screenList {
			return m, loadList()
		}
		if m.screen == screenBrowse {
			return m, loadBrowse(m.productF, m.kindF)
		}
		if m.screen == screenDoctor {
			return m, loadDoctor()
		}
	case "a":
		if m.screen == screenMarketplace {
			m.marketMode = "add"
			m.addInput.SetValue("")
			m.addInput.Focus()
			return m, nil
		}
	case "f":
		// cycle browse product filter
		if m.screen == screenBrowse {
			cycle := []string{"", "fest", "camp", "obey"}
			m.productF = nextIn(cycle, m.productF)
			return m, loadBrowse(m.productF, m.kindF)
		}
	case "c":
		if m.screen == screenBrowse {
			cycle := []string{"", "plugin", "tool", "product", "bundle"}
			m.kindF = nextIn(cycle, m.kindF)
			return m, loadBrowse(m.productF, m.kindF)
		}
	}
	return m, nil
}

func nextIn(opts []string, cur string) string {
	for i, o := range opts {
		if o == cur {
			return opts[(i+1)%len(opts)]
		}
	}
	return opts[0]
}

func (m model) maxCursor() int {
	switch m.screen {
	case screenHome:
		return len(homeItems) - 1
	case screenList, screenUninstall:
		n := len(m.list.Packages)
		if n == 0 {
			return 0
		}
		return n - 1
	case screenBrowse:
		n := len(m.browseFlat)
		if n == 0 {
			return 0
		}
		return n - 1
	case screenMarketplace:
		if m.marketMode == "add" {
			return 0
		}
		// list + actions: markets + refresh row
		n := len(m.markets) + 1 // last = refresh all
		if n < 1 {
			return 0
		}
		return n - 1
	case screenInstall:
		return 1 // channel row is left/right; enter installs
	default:
		return 0
	}
}

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenHome:
		return m.openHomeItem()
	case screenInstall:
		return m.startInstall()
	case screenUpdate:
		return m.startUpdate()
	case screenList:
		return m, nil
	case screenBrowse:
		return m.installBrowseSelection()
	case screenUninstall:
		if len(m.list.Packages) == 0 {
			return m, nil
		}
		pkg := m.list.Packages[m.cursor]
		m.confirmMsg = "Uninstall " + pkg.PackageID + "?"
		m.confirmYes = false
		m.confirmAct = "uninstall"
		m.confirmArg = pkg.PackageID
		m.screen = screenConfirm
		return m, nil
	case screenConfirm:
		if !m.confirmYes {
			m.screen = screenHome
			return m, nil
		}
		if m.confirmAct == "uninstall" {
			return m.startUninstall(m.confirmArg)
		}
		return m, nil
	case screenMarketplace:
		if m.marketMode == "add" {
			return m.submitMarketplaceAdd()
		}
		if m.cursor >= len(m.markets) {
			// refresh all
			return m, tea.Batch(func() tea.Msg {
				_, err := app.MarketplaceRefresh(context.Background(), "")
				if err != nil {
					return marketMsg{err: err}
				}
				views, err := app.MarketplaceList(context.Background())
				return marketMsg{views: views, err: err}
			})
		}
		// remove selected marketplace? use 'd' - for enter, refresh single
		name := m.markets[m.cursor].Name
		return m, tea.Batch(func() tea.Msg {
			_, err := app.MarketplaceRefresh(context.Background(), name)
			if err != nil {
				return marketMsg{err: err}
			}
			views, err := app.MarketplaceList(context.Background())
			return marketMsg{views: views, err: err}
		})
	case screenResult:
		m.screen = screenHome
		m.err = nil
		return m, nil
	case screenDoctor, screenShell:
		m.screen = screenHome
		return m, nil
	}
	return m, nil
}

func (m model) openHomeItem() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0:
		m.screen = screenInstall
		m.channelIdx = 0
		return m, nil
	case 1:
		m.screen = screenUpdate
		return m.startUpdate()
	case 2:
		m.screen = screenList
		return m, loadList()
	case 3:
		m.screen = screenBrowse
		m.productF, m.kindF = "", ""
		return m, loadBrowse("", "")
	case 4:
		m.screen = screenUninstall
		return m, loadList()
	case 5:
		m.screen = screenMarketplace
		m.marketMode = "list"
		return m, loadMarkets()
	case 6:
		m.screen = screenDoctor
		return m, loadDoctor()
	case 7:
		m.screen = screenShell
		bin, on, err := app.ManagedBinOnPath(context.Background())
		m.shellBin = bin
		m.shellOnPath = on
		if err != nil {
			m.err = err
		}
		snip, _ := app.ShellInit(context.Background(), "zsh")
		m.shellSnippet = snip
		return m, nil
	case 8:
		return m, tea.Quit
	}
	return m, nil
}

func (m model) startInstall() (tea.Model, tea.Cmd) {
	ch := m.channels[m.channelIdx]
	m.busy = true
	m.screen = screenProgress
	m.progress = app.ProgressEvent{Stage: "resolve", Percent: 0, Message: "starting install"}
	ctx, cancel := context.WithCancel(context.Background())
	m.opCancel = cancel
	return m, runInstall(ctx, ch)
}

func runInstall(ctx context.Context, channel string) tea.Cmd {
	return func() tea.Msg {
		prog := func(ev app.ProgressEvent) {
			// best-effort; bubbletea can't inject mid-cmd without program.Send
			// stage is still returned in final message body
			_ = ev
		}
		res, err := app.InstallFestival(ctx, app.InstallOptions{
			Channel:  channel,
			Verify:   source.DefaultVerifyOptions(nil, false),
			Progress: prog,
		})
		if err != nil {
			return opDoneMsg{title: "Install failed", body: err.Error(), err: err, success: false}
		}
		body := fmt.Sprintf("installed %s %s (%s)\n", res.Package, res.Version, res.Channel)
		for _, f := range res.Files {
			body += "  " + f + "\n"
		}
		return opDoneMsg{title: "Install complete", body: body, success: true}
	}
}

func (m model) startUpdate() (tea.Model, tea.Cmd) {
	m.busy = true
	m.screen = screenProgress
	m.progress = app.ProgressEvent{Stage: "resolve", Percent: 0.1, Message: "checking updates"}
	ctx, cancel := context.WithCancel(context.Background())
	m.opCancel = cancel
	return m, func() tea.Msg {
		res, warning, err := app.UpdateFestival(ctx, app.UpdateOptions{
			Verify: source.DefaultVerifyOptions(nil, false),
		})
		if err != nil {
			return opDoneMsg{title: "Update failed", body: err.Error(), err: err, success: false}
		}
		body := fmt.Sprintf("action: %s\nversion: %s\n", res.Action, res.Version)
		if res.From != "" {
			body += "from: " + res.From + "\n"
		}
		if warning != "" {
			body += "\n" + warning + "\n"
		}
		ok := res.Action == "upgraded" || res.Action == "current"
		title := "Update"
		switch res.Action {
		case "upgraded":
			title = "Updated"
		case "current":
			title = "Already current"
		case "unmanaged":
			title = "Unmanaged install"
			ok = false
		case "absent":
			title = "Not installed"
			ok = false
		}
		return opDoneMsg{title: title, body: body, success: ok}
	}
}

func (m model) startUninstall(packageID string) (tea.Model, tea.Cmd) {
	m.busy = true
	m.screen = screenProgress
	m.progress = app.ProgressEvent{Stage: "activate", Percent: 0.5, Message: "removing " + packageID}
	return m, func() tea.Msg {
		res, err := app.UninstallPackage(context.Background(), packageID)
		if err != nil {
			return opDoneMsg{title: "Uninstall failed", body: err.Error(), err: err, success: false}
		}
		body := res.Note
		if body == "" {
			body = "uninstalled " + res.Package + "\n"
			for _, f := range res.Removed {
				body += "  removed " + f + "\n"
			}
		}
		return opDoneMsg{title: "Uninstall complete", body: body, success: true}
	}
}

func (m model) installBrowseSelection() (tea.Model, tea.Cmd) {
	if len(m.browseFlat) == 0 {
		return m, nil
	}
	entry := m.browseFlat[m.cursor]
	// Map package ID segment to camp-/fest- selector when possible.
	target := entry.ID
	if i := strings.LastIndex(entry.ID, "/"); i >= 0 {
		seg := entry.ID[i+1:]
		if _, _, ok := app.PluginHost(seg); ok {
			target = seg
		} else if entry.Class == "bundle" || entry.Class == "product" || strings.Contains(entry.ID, "festival") {
			target = "festival"
		} else {
			// try installing by plugin short name if looks like camp-X/fest-X
			target = seg
		}
	}
	m.busy = true
	m.screen = screenProgress
	m.progress = app.ProgressEvent{Stage: "resolve", Percent: 0.1, Message: "installing " + entry.ID}
	return m, func() tea.Msg {
		res, err := app.InstallTarget(context.Background(), target, app.InstallOptions{
			Channel: "stable",
			Verify:  source.DefaultVerifyOptions(nil, false),
		})
		if err != nil {
			return opDoneMsg{title: "Install failed", body: err.Error() + "\n\n(selected " + entry.ID + " as " + target + ")", err: err, success: false}
		}
		body := fmt.Sprintf("installed %s %s\n", res.Package, res.Version)
		for _, f := range res.Files {
			body += "  " + f + "\n"
		}
		return opDoneMsg{title: "Install complete", body: body, success: true}
	}
}

func (m model) submitMarketplaceAdd() (tea.Model, tea.Cmd) {
	url := strings.TrimSpace(m.addInput.Value())
	if url == "" {
		m.err = fmt.Errorf("enter a git URL")
		return m, nil
	}
	m.marketMode = "list"
	return m, func() tea.Msg {
		_, err := app.MarketplaceAdd(context.Background(), url, "")
		if err != nil {
			return marketMsg{err: err}
		}
		views, err := app.MarketplaceList(context.Background())
		return marketMsg{views: views, err: err}
	}
}

func flattenBrowse(res app.BrowseResult) []app.BrowseEntry {
	seen := map[string]bool{}
	var out []app.BrowseEntry
	for _, g := range res.Groups {
		for _, p := range g.Packages {
			if seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			out = append(out, p)
		}
	}
	return out
}

func (m model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	// Keep chrome inside the frame; reserve a little margin for alt-screen edges.
	if w > 4 {
		w -= 0 // full width; lipgloss handles display width
	}
	s := m.styles
	var body string
	title := "home"
	footer := "↑↓ navigate  enter select  ? help  q quit"

	switch m.screen {
	case screenBoot:
		return anim.BootView(m.frame, w, s, m.reduced) + "\n" + s.Muted.Render("  enter to skip")
	case screenHome:
		title = "home"
		body = m.viewHome()
	case screenInstall:
		title = "install"
		body = m.viewInstall()
		footer = "←→ channel  enter install  esc back"
	case screenUpdate, screenProgress:
		title = "working"
		body = anim.ProgressFlame(m.progress.Percent, m.progress.Stage, m.progress.Message, m.frame, s)
		footer = "ctrl+c cancel"
	case screenList:
		title = "installed"
		body = m.viewList()
		footer = "r refresh  esc back"
	case screenBrowse:
		title = "browse"
		body = m.viewBrowse()
		footer = "f product  c class  enter install  r refresh  esc back"
	case screenUninstall:
		title = "uninstall"
		body = m.viewList()
		footer = "enter uninstall  esc back"
	case screenMarketplace:
		title = "marketplaces"
		body = m.viewMarketplace()
		footer = "a add  enter refresh  r reload  esc back"
	case screenDoctor:
		title = "doctor"
		body = m.viewDoctor()
		footer = "r re-run  esc back"
	case screenShell:
		title = "shell"
		body = m.viewShell()
		footer = "esc back"
	case screenConfirm:
		title = "confirm"
		body = components.ConfirmBox(m.confirmMsg, m.confirmYes, s)
		footer = "←→  y/n  enter"
	case screenResult:
		title = "result"
		body = m.viewResult()
		footer = "enter continue  esc home"
	}

	if m.help {
		body = components.HelpOverlay(s)
	}

	header := components.Header(title, m.opts.Version, w, s)
	foot := components.Footer(footer, w, s)
	parts := []string{header, body}
	if m.err != nil && m.screen != screenResult {
		parts = append(parts, components.ErrorBox(m.err, s))
	}
	parts = append(parts, foot)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) viewHome() string {
	s := m.styles
	var status string
	switch m.status.Action {
	case "managed":
		status = components.StatusLine(fmt.Sprintf("festival %s (%s) · managed", m.status.Version, m.status.Channel), "ok", s)
	case "unmanaged":
		status = components.StatusLine("camp/fest found but not managed by festival", "warn", s)
	case "absent":
		status = components.StatusLine("festival suite not installed", "fail", s)
	default:
		status = components.StatusLine("checking status…", "", s)
	}
	pathKind := "fail"
	pathText := "managed bin not on PATH"
	binShort := m.status.ManagedBin
	if len(binShort) > 48 {
		binShort = "…" + binShort[len(binShort)-47:]
	}
	if m.status.ManagedBinOnPath {
		pathKind = "ok"
		pathText = "PATH ok · " + binShort
	} else if m.status.ManagedBin != "" {
		pathText = "PATH missing · " + binShort
	}

	var flame string
	if m.reduced {
		flame = anim.StaticFlame(s)
	} else {
		flame = anim.Flame(m.frame, 1, s)
	}
	booths := anim.RenderBooths(anim.DefaultHomeBooths(homeBoothIndex(m.cursor)), m.frame, s)
	center := lipgloss.JoinVertical(lipgloss.Center, flame, "", booths)
	menu := components.Menu(homeItems, m.cursor, s)
	tag := s.Tagline.Render(anim.Tagline)

	return status + "\n" + components.StatusLine(pathText, pathKind, s) + "\n\n" +
		center + "\n\n" + menu + "\n" + tag
}

func (m model) viewInstall() string {
	s := m.styles
	var ch strings.Builder
	for i, c := range m.channels {
		if i == m.channelIdx {
			ch.WriteString(s.Selected.Render("▸ " + c + " "))
		} else {
			ch.WriteString(s.Muted.Render("  " + c + " "))
		}
	}
	return s.Title.Render("Install Festival suite (camp + fest)") + "\n\n" +
		s.Muted.Render("channel") + "\n" + ch.String() + "\n\n" +
		s.Fire.Render("enter") + s.Muted.Render(" to install")
}

func (m model) viewList() string {
	s := m.styles
	if len(m.list.Packages) == 0 {
		return s.Muted.Render("no packages installed yet")
	}
	items := make([]string, len(m.list.Packages))
	for i, p := range m.list.Packages {
		items[i] = fmt.Sprintf("%s  %s  (%s)", p.PackageID, p.Version, p.Channel)
	}
	return components.Menu(items, m.cursor, s)
}

func (m model) viewBrowse() string {
	s := m.styles
	filt := s.Muted.Render(fmt.Sprintf("filter product=%q class=%q", m.productF, m.kindF))
	if len(m.browseFlat) == 0 {
		return filt + "\n\n" + s.Muted.Render("no packages match (add a marketplace or refresh)")
	}
	items := make([]string, len(m.browseFlat))
	for i, p := range m.browseFlat {
		items[i] = fmt.Sprintf("%s  [%s]  %s", p.ID, p.Class, p.Source)
	}
	return filt + "\n\n" + components.Menu(items, m.cursor, s)
}

func (m model) viewMarketplace() string {
	s := m.styles
	if m.marketMode == "add" {
		return s.Title.Render("Add marketplace") + "\n\n" + m.addInput.View() + "\n\n" +
			s.Muted.Render("enter to add · esc cancel")
	}
	items := make([]string, 0, len(m.markets)+1)
	for _, v := range m.markets {
		line := fmt.Sprintf("%s  %s  pkgs=%d", v.Name, short(v.Commit, 12), v.Packages)
		if v.Err != "" {
			line += "  ERR:" + v.Err
		}
		items = append(items, line)
	}
	items = append(items, "↻ refresh all")
	return components.Menu(items, m.cursor, s) + "\n" + s.Muted.Render("a to add a git marketplace")
}

func (m model) viewDoctor() string {
	s := m.styles
	if len(m.checks) == 0 {
		return s.Muted.Render("running checks…")
	}
	var b strings.Builder
	// multi-booth style checks
	for i, c := range m.checks {
		spin := "·"
		if !m.reduced {
			spin = []string{"·", "°", "*", "✦"}[(m.frame+i)%4]
		}
		var line string
		switch c.Status {
		case "ok":
			line = s.OK.Render(fmt.Sprintf("[%s ok] ", spin)) + s.Normal.Render(c.ID+" — "+c.Message)
		case "warn":
			line = s.Warn.Render(fmt.Sprintf("[%s warn] ", spin)) + s.Normal.Render(c.ID+" — "+c.Message)
		default:
			line = s.Err.Render(fmt.Sprintf("[%s fail] ", spin)) + s.Normal.Render(c.ID+" — "+c.Message)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func (m model) viewShell() string {
	s := m.styles
	status := "NOT on PATH"
	kind := "fail"
	if m.shellOnPath {
		status = "on PATH"
		kind = "ok"
	}
	return components.StatusLine(m.shellBin+" · "+status, kind, s) + "\n\n" +
		s.Muted.Render("add to your shell rc:") + "\n\n" +
		s.FireTip.Render(m.shellSnippet)
}

func (m model) viewResult() string {
	s := m.styles
	title := s.Title.Render(m.resultTitle)
	if m.resultOK {
		if !m.reduced {
			return title + "\n\n" + anim.Celebrate(m.frame, s) + "\n\n" + s.Normal.Render(m.resultBody)
		}
		return title + "\n\n" + s.OK.Render("done") + "\n\n" + s.Normal.Render(m.resultBody)
	}
	return title + "\n\n" + s.Err.Render(m.resultBody)
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// homeBoothIndex maps the home menu cursor onto ambient booths so the
// multi-activity strip tracks what the user is looking at.
func homeBoothIndex(cursor int) int {
	switch cursor {
	case 0, 1: // install / update
		return 0
	case 2, 3, 4: // list / browse / uninstall
		return 1
	case 5: // marketplaces
		return 2
	case 6: // doctor
		return 3
	case 7: // shell / path
		return 4
	default:
		return 0
	}
}
