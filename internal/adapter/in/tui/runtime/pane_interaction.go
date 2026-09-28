package runtime

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/questionbridge"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/runtimeui"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/mentionview"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	questionpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/question"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	questiontool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/question"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

type paneActionKind uint8

const (
	paneActionNone paneActionKind = iota
	paneActionClose
	paneActionAcceptSlash
	paneActionAcceptMention
	paneActionToggleSkill
	paneActionReloadModels
	paneActionApplyModelSetup
	paneActionOpenProviderSelect
	paneActionOpenProviderEditor
	paneActionPermissionActivity
	paneActionPermissionResolve
	paneActionSetPermissionMode
	paneActionSetLowConcurrency
	paneActionResumeSession
	paneActionScrollLines
	paneActionScrollPage
	paneActionProviderDelete
	paneActionProviderActivate
	paneActionProviderModels
	paneActionProviderEdit
	paneActionProviderFetch
	paneActionProviderSave
	paneActionQuestionResolve
)

type paneAction struct {
	kind             paneActionKind
	paneID           string
	sessionID        string
	reasoning        domain.ReasoningEffort
	runSlash         bool
	skillName        string
	providerName     string
	modelID          string
	activity         string
	permission       permissionOption
	permissionMode   permissionModeChoice
	lowConcurrency   model.LowConcurrencySetting
	scrollLines      int
	key              tea.KeyPressMsg
	providerItem     providerSelectItem
	providerSave     providerSaveRequest
	questionResponse questiontool.Response
}

type paneKeyResult struct {
	handled bool
	cmd     tea.Cmd
	action  paneAction
}

type isolatedPaneKeyHandler interface {
	HandlePaneKey(paneRenderContext, tea.KeyPressMsg) paneKeyResult
}

type isolatedPanePasteHandler interface {
	HandlePanePaste(paneRenderContext, tea.PasteMsg) paneKeyResult
}

type isolatedPaneMsgHandler interface {
	HandlePaneMsg(paneRenderContext, tea.Msg) paneKeyResult
}

func (m *bubbleModel) applyPaneAction(action paneAction) tea.Cmd {
	switch action.kind {
	case paneActionClose:
		m.panes.bottom.remove(action.paneID)
	case paneActionAcceptSlash:
		_, cmd := m.acceptSlash(action.runSlash)
		return cmd
	case paneActionAcceptMention:
		_, cmd := m.acceptMention()
		return cmd
	case paneActionToggleSkill:
		if m.skills == nil {
			return nil
		}
		active, err := m.skills.Toggle(action.skillName)
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		m.persistActiveSkills()
		state := "Deactivated"
		if active {
			state = "Activated"
		}
		noticeCmd := m.showTransientNotice(fmt.Sprintf("%s skill %q", state, action.skillName))
		if view, _ := m.panes.bottom.find(skillsViewID).(*skillsPaneView); view != nil {
			refreshCmd := view.refreshItems(newPaneRenderContext(m))
			return tea.Batch(noticeCmd, refreshCmd)
		}
		return noticeCmd
	case paneActionReloadModels:
		if view, _ := m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView); view != nil {
			return view.loadProvider(m, action.runSlash)
		}
	case paneActionApplyModelSetup:
		m.panes.bottom.remove(modelSetupViewID)
		return m.beginModelSetupSelect(action.providerName, action.modelID, action.reasoning, false)
	case paneActionOpenProviderSelect:
		m.panes.bottom.remove(modelSetupViewID)
		if !m.panes.bottom.has(providerSelectViewID) {
			m.panes.bottom.push(newProviderSelectPaneView(m))
		}
	case paneActionOpenProviderEditor:
		m.panes.bottom.remove(modelSetupViewID)
		m.panes.bottom.remove(providerSelectViewID)
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneView())
		}
	case paneActionPermissionActivity:
		m.activity = action.activity
	case paneActionPermissionResolve:
		return m.resolvePermission(action.permission)
	case paneActionQuestionResolve:
		return m.resolveQuestion(action.questionResponse)
	case paneActionSetPermissionMode:
		m.panes.bottom.remove(permissionModeViewID)
		m.applyPermissionModeChoice(action.permissionMode)
	case paneActionSetLowConcurrency:
		m.panes.bottom.remove(lowConcurrencyViewID)
		return m.applyLowConcurrencySetting(action.lowConcurrency)
	case paneActionResumeSession:
		m.panes.bottom.remove(sessionResumeViewID)
		return m.resumeSession(action.sessionID)
	case paneActionScrollLines:
		m.scrollConversationLines(action.scrollLines)
	case paneActionScrollPage:
		return m.updateConversationViewport(action.key)
	case paneActionProviderDelete:
		m.panes.bottom.remove(providerSelectViewID)
		return m.beginProviderDelete(action.providerItem.name)
	case paneActionProviderActivate:
		m.panes.bottom.remove(providerSelectViewID)
		return m.beginProviderSelect(action.providerItem.name)
	case paneActionProviderModels:
		m.panes.bottom.remove(providerSelectViewID)
		if !m.panes.bottom.has(modelSetupViewID) {
			mv := newModelSetupPaneView(m)
			for i, name := range mv.providerNames {
				if strings.EqualFold(name, action.providerItem.name) {
					mv.providerIndex = i
					break
				}
			}
			m.panes.bottom.push(mv)
			if _, ok := m.providers[strings.ToLower(action.providerItem.name)]; ok {
				return mv.loadProvider(m, false)
			}
		}
	case paneActionProviderEdit:
		m.panes.bottom.remove(providerSelectViewID)
		if !m.panes.bottom.has(providerViewID) {
			item := action.providerItem
			if item.isConfigured {
				if cfg, ok := m.providers[strings.ToLower(item.name)]; ok {
					pv := newProviderPaneViewWithConfig(cfg)
					pv.activateOnSave = item.isActive
					m.pushProviderPane(pv)
				} else {
					pv := newProviderPaneViewWithPreset(item.name)
					pv.activateOnSave = item.isActive
					m.pushProviderPane(pv)
				}
			} else if item.kind == providerItemPreset {
				m.pushProviderPane(newProviderPaneViewWithPreset(item.presetID))
			} else {
				m.pushProviderPane(newProviderPaneView())
			}
		}
	case paneActionProviderFetch:
		if view, _ := m.panes.bottom.find(providerViewID).(*providerPaneView); view != nil {
			return view.beginFetch(m.ctx, m.application.Models, m.runtimeConfig.ModelDiscoveryTimeout)
		}
	case paneActionProviderSave:
		return m.beginProviderSave(action.providerSave)
	}
	return nil
}

type paneRenderContext struct {
	width              int
	height             int
	activeModel        string
	spinner            string
	projectTrusted     bool
	hasWorkDir         bool
	workDir            string
	skillItems         []skillListItem
	todos              []tododomain.Item
	slashMatches       []slashCommand
	mentionMatches     []mentionview.Item
	agentSnapshot      []agent.AgentStatus
	agentActivity      map[string]string
	subagentsEnabled   bool
	keyboardCapability keyboardCapability
	providers          map[string]config.ProviderConfig
}

func newPaneRenderContext(m *bubbleModel) paneRenderContext {
	ctx := paneRenderContext{width: defaultBubbleWidth, height: defaultBubbleHeight}
	if m == nil {
		return ctx
	}
	ctx.width = m.layout.width
	ctx.height = m.layout.height
	ctx.activeModel = m.activeModel
	ctx.spinner = m.spinnerIndicator()
	ctx.projectTrusted = m.projectTrusted
	ctx.hasWorkDir = m.workDir != ""
	ctx.workDir = m.workDir
	ctx.todos = tododomain.CloneItems(m.todo)
	ctx.slashMatches = append([]slashCommand(nil), m.slashMatches()...)
	ctx.mentionMatches = append([]mentionview.Item(nil), m.mentionMatches()...)
	ctx.agentSnapshot = append([]agent.AgentStatus(nil), m.agentSnapshot...)
	ctx.subagentsEnabled = m.subagentsEnabled
	ctx.keyboardCapability = m.keyboardCapability
	ctx.providers = make(map[string]config.ProviderConfig, len(m.providers))
	for name, cfg := range m.providers {
		ctx.providers[name] = cfg
	}
	ctx.agentActivity = make(map[string]string, len(m.agentActivity))
	for id, state := range m.agentActivity {
		ctx.agentActivity[id] = state.String()
	}
	if m.skills != nil {
		for _, item := range m.skills.List() {
			ctx.skillItems = append(ctx.skillItems, skillListItem{
				name:        item.Name,
				description: item.Description,
				scope:       string(item.Scope),
				active:      m.skills.IsActivated(item.Name),
			})
		}
	}

	return ctx
}

// --- Question Pane ---

const questionViewID = "question"

type questionRequest = questionbridge.Request
type questionRequestMsg = questionbridge.RequestMsg

type questionPaneView struct {
	pending         questionRequest
	items           []questiontool.QuestionItem
	currentQuestion int
	answers         []questiontool.AnswerItem
	index           int
	selected        map[int]bool
	writeInMode     bool
	writeInText     string
	tone            panecommon.Tone
}

func (*questionPaneView) ID() string                             { return questionViewID }
func (*questionPaneView) PresentationMode() panePresentationMode { return paneBlocking }

func (v *questionPaneView) Render(ctx paneRenderContext) string {
	return v.card(ctx)
}

func (v *questionPaneView) initQuestionState() {
	v.index = 0
	v.selected = make(map[int]bool)
	v.writeInMode = false
	v.writeInText = ""
	if v.currentQuestion < len(v.items) {
		item := v.items[v.currentQuestion]
		if item.Recommended != "" {
			for i, opt := range item.Options {
				if opt == item.Recommended || strings.EqualFold(opt, item.Recommended) {
					v.index = i
					break
				}
			}
		} else {
			for i, opt := range item.Options {
				if strings.Contains(opt, "(Recommended)") {
					v.index = i
					break
				}
			}
		}
	}
}

func (v *questionPaneView) card(ctx paneRenderContext) string {
	if len(v.items) == 0 {
		return ""
	}
	item := v.items[v.currentQuestion]
	result := questionpane.QuestionView(questionpane.QuestionSnapshot{
		Width:          ctx.width,
		Height:         ctx.height,
		Question:       item.Question,
		Options:        item.Options,
		Multiple:       item.Multiple,
		Recommended:    item.Recommended,
		QuestionIndex:  v.currentQuestion,
		TotalQuestions: len(v.items),
		Index:          v.index,
		Selected:       v.selected,
		WriteInMode:    v.writeInMode,
		WriteInText:    v.writeInText,
		Tone:           v.tone,
	})

	return renderModalRows(ctx, paneToneColor(result.Tone), result.Rows)
}

func (v *questionPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	if len(v.items) == 0 {
		return paneKeyResult{handled: true}
	}
	item := v.items[v.currentQuestion]
	hasOptions := len(item.Options) > 0

	resolveDeclined := func() paneKeyResult {
		return paneKeyResult{
			handled: true,
			action: paneAction{
				kind: paneActionQuestionResolve,
				questionResponse: questiontool.Response{
					Status: questiontool.StatusDeclined,
					Answer: "User declined to answer",
				},
			},
		}
	}

	recordAnswerAndAdvance := func(ans string, selected []string) paneKeyResult {
		ansItem := questiontool.AnswerItem{
			Question:        item.Question,
			Answer:          ans,
			SelectedOptions: selected,
		}
		if v.currentQuestion < len(v.answers) {
			v.answers[v.currentQuestion] = ansItem
		} else {
			v.answers = append(v.answers, ansItem)
		}

		if v.currentQuestion < len(v.items)-1 {
			v.currentQuestion++
			v.initQuestionState()
			return paneKeyResult{handled: true}
		}

		var finalAnswer string
		if len(v.answers) == 1 {
			finalAnswer = v.answers[0].Answer
		} else {
			var sb strings.Builder
			for i, a := range v.answers {
				if i > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("%d. %s: %s", i+1, a.Question, a.Answer))
			}
			finalAnswer = sb.String()
		}
		primarySelected := selected
		if len(v.answers) > 0 {
			primarySelected = v.answers[0].SelectedOptions
		}
		return paneKeyResult{
			handled: true,
			action: paneAction{
				kind: paneActionQuestionResolve,
				questionResponse: questiontool.Response{
					Status:          questiontool.StatusAnswered,
					Answer:          finalAnswer,
					SelectedOptions: primarySelected,
					Answers:         v.answers,
				},
			},
		}
	}

	backtrack := func() paneKeyResult {
		if v.currentQuestion > 0 {
			v.currentQuestion--
			v.initQuestionState()
			return paneKeyResult{handled: true}
		}
		return resolveDeclined()
	}

	if v.writeInMode || !hasOptions {
		switch {
		case key.Matches(message, paneutil.Keys.Escape):
			if hasOptions {
				v.writeInMode = false
				return paneKeyResult{handled: true}
			}
			return backtrack()
		case key.Matches(message, paneutil.Keys.Confirm):
			text := strings.TrimSpace(v.writeInText)
			if text == "" {
				if hasOptions {
					v.writeInMode = false
					return paneKeyResult{handled: true}
				}
				return backtrack()
			}
			return recordAnswerAndAdvance(text, nil)
		case message.String() == "backspace":
			if len(v.writeInText) > 0 {
				r := []rune(v.writeInText)
				v.writeInText = string(r[:len(r)-1])
			}
			return paneKeyResult{handled: true}
		case message.Code == ' ' || message.Text == " ":
			v.writeInText += " "
			return paneKeyResult{handled: true}
		default:
			if text := message.Text; len(text) > 0 {
				v.writeInText += text
				return paneKeyResult{handled: true}
			}
			return paneKeyResult{handled: true}
		}
	}

	// Normal options mode
	totalItems := len(item.Options) + 1 // options + write-in option
	if v.index >= totalItems {
		v.index = totalItems - 1
	}
	if v.index < 0 {
		v.index = 0
	}

	switch {
	case key.Matches(message, paneutil.Keys.Escape) || message.String() == "left":
		return backtrack()
	case key.Matches(message, paneutil.Keys.Up):
		if v.index > 0 {
			v.index--
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Down):
		if v.index < totalItems-1 {
			v.index++
		}
		return paneKeyResult{handled: true}
	case message.Text == "w" || message.Text == "W":
		v.writeInMode = true
		return paneKeyResult{handled: true}
	case message.Text >= "1" && message.Text <= "9":
		idx := int(message.Text[0] - '1')
		if idx < len(item.Options) {
			if item.Multiple {
				if v.selected == nil {
					v.selected = make(map[int]bool)
				}
				v.selected[idx] = !v.selected[idx]
				v.index = idx
				return paneKeyResult{handled: true}
			}
			opt := item.Options[idx]
			return recordAnswerAndAdvance(opt, []string{opt})
		}
		return paneKeyResult{handled: true}
	case message.Text == " ":
		if item.Multiple && v.index < len(item.Options) {
			if v.selected == nil {
				v.selected = make(map[int]bool)
			}
			v.selected[v.index] = !v.selected[v.index]
			return paneKeyResult{handled: true}
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Confirm):
		if v.index == len(item.Options) {
			v.writeInMode = true
			return paneKeyResult{handled: true}
		}
		if item.Multiple {
			selected := make([]string, 0)
			for i, opt := range item.Options {
				if v.selected != nil && v.selected[i] {
					selected = append(selected, opt)
				}
			}
			if len(selected) == 0 && v.index < len(item.Options) {
				selected = append(selected, item.Options[v.index])
			}
			ans := strings.Join(selected, ", ")
			return recordAnswerAndAdvance(ans, selected)
		}
		opt := item.Options[v.index]
		return recordAnswerAndAdvance(opt, []string{opt})
	default:
		return paneKeyResult{handled: true}
	}
}

func (m *bubbleModel) questionView() *questionPaneView {
	if m.panes.bottom == nil {
		return nil
	}
	view, _ := m.panes.bottom.find(questionViewID).(*questionPaneView)
	return view
}

func (m *bubbleModel) openQuestion(request questionRequest) {
	if m.panes.bottom == nil {
		return
	}
	items := request.Request.NormalizedItems()
	view := &questionPaneView{
		pending:         request,
		items:           items,
		currentQuestion: 0,
		answers:         make([]questiontool.AnswerItem, 0, len(items)),
		selected:        make(map[int]bool),
		tone:            panecommon.ToneUser,
	}
	view.initQuestionState()
	m.panes.bottom.push(view)
	m.requestRelayout()
	m.reconcileLayout()
	if !runtimeui.IsWaitingForQuestion(m.activity) {
		m.pendingActivity = m.activity
	}
	m.activity = runtimeui.ActivityWaitingForQuestion
}

func (m *bubbleModel) resolveQuestion(response questiontool.Response) tea.Cmd {
	view := m.questionView()
	if view == nil {
		return nil
	}
	view.pending.Respond(response)
	m.panes.bottom.remove(questionViewID)
	m.requestRelayout()
	m.reconcileLayout()
	m.activity = m.pendingActivity
	if m.activity == "" || runtimeui.IsWaitingForQuestion(m.activity) {
		if m.busy {
			m.activity = "running tool"
		} else {
			m.activity = runtimeui.ActivityReady
		}
	}
	return nil
}
