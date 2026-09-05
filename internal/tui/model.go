package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/launch"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/tui/theme"
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
	screenLaunchpad
	screenHelp
	screenProgress
	screenResult
	screenConfirm
	screenChildOutput
)

type tickMsg time.Time

type statusMsg struct {
	sum app.StatusSummary
	err error
}

type latestMsg struct {
	latest string
}

// packageUpgradeMsg means Update found a newer package-manager suite and the
// hub should drop the alt-screen and run origin.Upgrade on the real TTY.
type packageUpgradeMsg struct {
	spec launch.Spec
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

// opDoneMsg is an operation's final result. It carries the operation's
// progressStream so completion can release the live stream directly instead
// of depending on progressClosedMsg arriving first.
type opDoneMsg struct {
	stream  *progressStream
	title   string
	body    string
	err     error
	success bool
	// restart is true when the result screen should offer a hub restart
	// (the update just replaced the running festival binary).
	restart bool
}

// consentNeededMsg is returned when a strict install/update hit E_UNVERIFIED_REFUSED.
// The model opens an explicit override dialog before retrying with AllowUnverified.
type consentNeededMsg struct {
	action string
	cause  error
}

// progressMsg is one event drained from an operation's progressStream. The
// stream identifies the producer so events from a superseded operation are
// dropped instead of driving the current bar.
type progressMsg struct {
	stream *progressStream
	ev     app.ProgressEvent
}

// progressClosedMsg ends the drain loop: its stream produced its last event.
type progressClosedMsg struct {
	stream *progressStream
}

// childChunkMsg carries one piped output chunk from a capture-mode child.
type childChunkMsg []byte

// childExitMsg is the capture child's final result, after all output chunks.
type childExitMsg launch.Result

type model struct {
	// ctx is the RunLoop program context. Every tea.Cmd that performs network
	// or git work derives from it so quitting cancels in-flight operations.
	ctx       context.Context
	opts      Options
	styles    theme.Styles
	reduced   bool
	width     int
	height    int
	screen    screen
	cursor    int
	frame     int
	bootLeft  int // frames remaining on splash
	status    app.StatusSummary
	statusErr error
	err       error
	quitErr   error
	help      bool

	// install
	channelIdx   int
	channels     []string
	installKind  string // "", "package", "leftover"
	installForce bool

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
	// warnText is unverified-content (and similar) warnings captured from the
	// in-flight operation's WarnWriter. Shown on the working screen and
	// copied onto the result screen. Never written to stderr while the
	// alt-screen is live.
	warnText string
	// resultRestart is true when the result screen should offer to restart
	// the hub (an update just replaced the running festival binary).
	resultRestart bool

	// progressStream is the in-flight operation's event channel, drained by
	// waitProgress. Nil when no operation is reporting.
	progressStream *progressStream

	// confirm
	confirmMsg string
	confirmYes bool
	confirmAct string // uninstall | install-unverified | update-unverified | browse-install-unverified
	confirmArg string

	// op in flight cancel
	opCancel context.CancelFunc
	busy     bool

	// hub session handoff: set before tea.Quit so RunLoop can spawn a child
	pendingLaunch *launch.Spec
	// soft status after returning from a child tool
	banner string

	// launchpad
	launchEntries []launch.Entry

	// launchpad capture: oneshot/stream entries run inside the hub with
	// piped output instead of owning the TTY. captureOut is bounded
	// scrollback; never a strings.Builder (models are copied by value).
	capture      *launch.Capture
	captureTitle string
	captureMode  launch.Mode
	captureOut   []byte
	captureRes   *launch.Result
	captureVP    viewport.Model
}

// captureMaxBytes bounds capture scrollback; the head is trimmed past this.
const captureMaxBytes = 512 * 1024

// homeItemID names a home menu entry independently of its position. Every
// lookup of "what does this row do" goes through an id, so inserting or
// reordering entries cannot silently rewire the menu, which a switch over
// cursor indices did without any test noticing.
type homeItemID string

const (
	homeInstall     homeItemID = "install"
	homeUpdate      homeItemID = "update"
	homeList        homeItemID = "list"
	homeBrowse      homeItemID = "browse"
	homeUninstall   homeItemID = "uninstall"
	homeMarketplace homeItemID = "marketplace"
	homeDoctor      homeItemID = "doctor"
	homeShell       homeItemID = "shell"
	homeLaunchpad   homeItemID = "launchpad"
	homeQuit        homeItemID = "quit"
)

// homeItem is one home menu row. booth is the index into
// anim.DefaultHomeBooths that lights up while the row is selected, so the
// ambient strip tracks the menu without a second position table to keep in
// sync.
type homeItem struct {
	id    homeItemID
	label string
	booth int
}

func (m model) homeMenu() []homeItem {
	items := []homeItem{
		{id: homeInstall, label: "Install Festival suite", booth: 0},
		{id: homeUpdate, label: "Update Festival", booth: 0},
		{id: homeList, label: "Installed packages", booth: 1},
		{id: homeBrowse, label: "Browse catalog", booth: 1},
		{id: homeUninstall, label: "Uninstall package", booth: 1},
		{id: homeMarketplace, label: "Marketplaces", booth: 2},
		{id: homeDoctor, label: "Doctor", booth: 3},
		{id: homeShell, label: "Shell / PATH setup", booth: 4},
		{id: homeLaunchpad, label: "Launchpad (camp / fest tools)", booth: 0},
		{id: homeQuit, label: "Quit", booth: 0},
	}
	if m.status.Action == "package" || m.status.Dual {
		items[m.indexOfIn(items, homeInstall)].label = "How you installed"
	}
	if m.updateAvailable() {
		items[m.indexOfIn(items, homeUpdate)].label = "Update Festival · " + m.status.Latest + " available"
	}
	return items
}

func (m model) homeItems() []string {
	menu := m.homeMenu()
	labels := make([]string, len(menu))
	for i, it := range menu {
		labels[i] = it.label
	}
	return labels
}

// homeIndexOf is the cursor position of id, or -1 when the menu does not
// currently carry that entry.
func (m model) homeIndexOf(id homeItemID) int {
	return m.indexOfIn(m.homeMenu(), id)
}

func (m model) indexOfIn(items []homeItem, id homeItemID) int {
	for i, it := range items {
		if it.id == id {
			return i
		}
	}
	return -1
}

// homeItemAt is the entry under a cursor position, guarding the bounds so a
// stale cursor cannot index out of the menu.
func (m model) homeItemAt(cursor int) (homeItem, bool) {
	menu := m.homeMenu()
	if cursor < 0 || cursor >= len(menu) {
		return homeItem{}, false
	}
	return menu[cursor], true
}

func (m model) updateAvailable() bool {
	ver := strings.TrimPrefix(m.status.Version, "v")
	latest := strings.TrimPrefix(m.status.Latest, "v")
	return latest != "" && ver != "" && installer.VersionLess(ver, latest)
}

func (m model) defaultHomeCursor() int {
	if m.updateAvailable() || (m.status.Action == "managed" && !m.status.Dual) {
		if i := m.homeIndexOf(homeUpdate); i >= 0 {
			return i
		}
	}
	return 0
}

// moveToUpdateEntry parks the cursor on Update once a newer release is known,
// but only when the user has not moved it off the entry the home screen opened
// on. Nudging a cursor the user placed themselves would be rude.
func (m model) moveToUpdateEntry(cursor int) int {
	update := m.homeIndexOf(homeUpdate)
	if update < 0 {
		return cursor
	}
	if cursor == 0 || cursor == m.homeIndexOf(homeInstall) {
		return update
	}
	return cursor
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
		ctx:           context.Background(),
		opts:          opts,
		styles:        theme.New(),
		reduced:       theme.ReducedMotion(),
		screen:        screenBoot,
		bootLeft:      12, // ~1.2s at 100ms
		channels:      []string{"stable", "rc", "dev"},
		channelIdx:    0,
		addInput:      ti,
		width:         80,
		height:        24,
		launchEntries: launch.Catalog(),
	}
	if m.reduced {
		m.bootLeft = 1
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.loadStatus())
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) opContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(m.ctx)
}

func (m model) loadStatus() tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		sum, err := app.Status(ctx)
		return statusMsg{sum: sum, err: err}
	}
}

func (m model) shouldCheckLatest() bool {
	return m.status.Action == "package" || m.status.Action == "managed" || m.status.Dual
}

func (m model) loadLatest() tea.Cmd {
	ctx := m.ctx
	channel := m.status.Channel
	if channel == "" {
		channel = "stable"
	}
	return func() tea.Msg {
		latest, err := app.LookupLatestSuite(ctx, channel)
		if err != nil {
			return latestMsg{}
		}
		return latestMsg{latest: latest}
	}
}

func (m model) loadList() tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		res, err := app.ListInstalled(ctx)
		return listMsg{res: res, err: err}
	}
}

func (m model) loadBrowse(product, kind string) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		res, err := app.Browse(ctx, app.BrowseOptions{
			Product: product,
			Kind:    kind,
			Verify:  tuiVerifyOptions(nil, false),
		})
		return browseMsg{res: res, err: err}
	}
}

func (m model) loadDoctor() tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		return doctorMsg{checks: app.Doctor(ctx)}
	}
}

var (
	marketplaceListFn = app.MarketplaceListExisting
	marketplaceSeedFn = app.MarketplaceSeedOfficial
)

func (m model) loadMarkets() tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		views, err := marketplaceListFn(ctx, tuiVerifyOptions(nil, false))
		return marketMsg{views: views, err: err}
	}
}

func (m model) seedOfficialMarketplace() tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		err := marketplaceSeedFn(ctx, tuiVerifyOptions(nil, false))
		views, listErr := marketplaceListFn(ctx, tuiVerifyOptions(nil, false))
		if err == nil {
			err = listErr
		}
		return marketMsg{views: views, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.captureVP.Width = captureVPWidth(msg.Width)
		m.captureVP.Height = captureVPHeight(msg.Height)
		if m.captureOut != nil {
			m.captureVP.SetContent(captureRender(m.captureOut, m.captureVP.Width))
		}
		return m, nil

	case childChunkMsg:
		if m.capture == nil {
			return m, nil
		}
		follow := m.captureVP.AtBottom()
		m.captureOut = append(m.captureOut, msg...)
		if len(m.captureOut) > captureMaxBytes {
			m.captureOut = m.captureOut[len(m.captureOut)-captureMaxBytes:]
		}
		m.captureVP.SetContent(captureRender(m.captureOut, m.captureVP.Width))
		if follow {
			m.captureVP.GotoBottom()
		}
		return m, captureNext(m.capture)

	case childExitMsg:
		res := launch.Result(msg)
		m.captureRes = &res
		m.capture = nil
		m.captureVP.SetContent(captureRender(m.captureOut, m.captureVP.Width))
		return m, nil

	case tickMsg:
		m.frame++
		if m.screen == screenBoot {
			m.bootLeft--
			if m.bootLeft <= 0 {
				m.screen = screenHome
				m.cursor = m.defaultHomeCursor()
			}
		}
		return m, tickCmd()

	case statusMsg:
		m.status = msg.sum
		m.statusErr = msg.err
		if m.screen == screenBoot || m.screen == screenHome {
			m.cursor = m.defaultHomeCursor()
		}
		if m.shouldCheckLatest() {
			return m, m.loadLatest()
		}
		return m, nil

	case latestMsg:
		m.status.Latest = strings.TrimPrefix(msg.latest, "v")
		if (m.screen == screenHome || m.screen == screenBoot) && m.updateAvailable() {
			m.cursor = m.moveToUpdateEntry(m.cursor)
		}
		return m, nil

	case packageUpgradeMsg:
		m.pendingLaunch = &msg.spec
		return m, tea.Quit

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
		return m.applyProgress(msg)

	case progressClosedMsg:
		if msg.stream == m.progressStream {
			m.progressStream = nil
		}
		return m, nil

	case opDoneMsg:
		return m.applyOpDone(msg)

	case consentNeededMsg:
		m.busy = false
		if m.opCancel != nil {
			m.opCancel()
			m.opCancel = nil
		}
		m.confirmMsg = "Marketplace metadata is unsigned. Continue with the unverified-content override?"
		m.confirmYes = false
		m.confirmAct = msg.action
		m.confirmArg = ""
		m.err = msg.cause
		m.screen = screenConfirm
		return m, nil

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

	// Text fields must capture keys before the global switch, or URLs containing
	// q or ? fire the quit/help shortcuts instead of being typed.
	if m.textInputActive() {
		return m.handleTextInputKey(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		if m.screen == screenChildOutput && m.capture != nil {
			m.capture.Stop()
			return m, nil
		}
		if m.busy && m.opCancel != nil {
			m.opCancel()
			m.busy = false
			m.screen = screenHome
			return m, nil
		}
		return m, tea.Quit
	case "q":
		if m.screen == screenChildOutput {
			return m.closeChildOutput()
		}
		if m.screen == screenHome || m.screen == screenBoot {
			return m, tea.Quit
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
			m.cursor = m.defaultHomeCursor()
			return m, nil
		}
		if m.screen == screenChildOutput {
			return m.closeChildOutput()
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
			m.cursor = m.defaultHomeCursor()
			return m, nil
		}
		return m.handleEnter()
	}

	// Capture screen: remaining keys scroll the output viewport.
	if m.screen == screenChildOutput {
		var cmd tea.Cmd
		m.captureVP, cmd = m.captureVP.Update(msg)
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
	case "0":
		// Digit 0 is Quit, whatever position Quit currently holds. Digits 1
		// through 9 address the first nine entries.
		if m.screen == screenHome {
			if i := m.homeIndexOf(homeQuit); i >= 0 {
				m.cursor = i
			}
			return m.handleEnter()
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if m.screen == screenHome {
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(m.homeItems()) {
				m.cursor = idx
				return m.handleEnter()
			}
		}
	case "r":
		if m.screen == screenMarketplace {
			return m, m.loadMarkets()
		}
		if m.screen == screenList {
			return m, m.loadList()
		}
		if m.screen == screenBrowse {
			return m, m.loadBrowse(m.productF, m.kindF)
		}
		if m.screen == screenDoctor {
			return m, m.loadDoctor()
		}
		if m.screen == screenResult && m.resultRestart {
			return m.restartHub()
		}
	case "a":
		if m.screen == screenMarketplace {
			m.marketMode = "add"
			m.addInput.SetValue("")
			m.addInput.Focus()
			return m, nil
		}
	case "s":
		if m.screen == screenMarketplace && m.marketMode != "add" {
			return m, m.seedOfficialMarketplace()
		}
	case "f":
		if m.screen == screenInstall && m.installKind == "package" {
			m.installKind = ""
			m.installForce = true
			return m, nil
		}
		// cycle browse product filter
		if m.screen == screenBrowse {
			cycle := []string{"", "fest", "camp", "obey"}
			m.productF = nextIn(cycle, m.productF)
			return m, m.loadBrowse(m.productF, m.kindF)
		}
	case "c":
		if m.screen == screenBrowse {
			cycle := []string{"", "plugin", "tool", "product", "bundle"}
			m.kindF = nextIn(cycle, m.kindF)
			return m, m.loadBrowse(m.productF, m.kindF)
		}
	}
	return m, nil
}

func (m model) textInputActive() bool {
	return m.screen == screenMarketplace && m.marketMode == "add"
}

func (m model) handleTextInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.marketMode = "list"
		return m, nil
	case "enter":
		return m.submitMarketplaceAdd()
	}
	var cmd tea.Cmd
	m.addInput, cmd = m.addInput.Update(msg)
	return m, cmd
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
		return len(m.homeItems()) - 1
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
	case screenLaunchpad:
		n := len(m.launchEntries)
		if n == 0 {
			return 0
		}
		return n - 1
	default:
		return 0
	}
}
