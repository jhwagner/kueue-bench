package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/jhwagner/kueue-bench/pkg/watcher"
	"github.com/jhwagner/kueue-bench/pkg/workload"
)

// submitResultMsg carries the result of a workload submission attempt.
type submitResultMsg struct {
	err error
}

// --- Field indices -----------------------------------------------------------

// The field list is dynamic: fieldCPU/fieldMem/fieldGPU are only in tab order
// when Size=Custom; fieldReplicas only when Type=JobSet.
const (
	fieldType     = 0
	fieldQueue    = 1
	fieldSize     = 2
	fieldCPU      = 3
	fieldMem      = 4
	fieldGPU      = 5
	fieldReplicas = 6
	fieldDuration = 7
	fieldPriority = 8
	fieldSubmit   = 9
	fieldCount    = 10
)

// --- Size presets ------------------------------------------------------------

type sizePreset struct {
	label  string
	cpu    resource.Quantity
	mem    resource.Quantity
	gpu    resource.Quantity
}

var sizePresets = []sizePreset{
	{label: "S", cpu: resource.MustParse("1"), mem: resource.MustParse("4Gi")},
	{label: "M", cpu: resource.MustParse("4"), mem: resource.MustParse("16Gi")},
	{label: "L", cpu: resource.MustParse("8"), mem: resource.MustParse("64Gi"), gpu: resource.MustParse("1")},
	{label: "Custom"},
}

const sizeCustom = 3

// --- Model -------------------------------------------------------------------

// submitViewModel is the modal workload-submission dialog.
type submitViewModel struct {
	// Option selectors (index into their respective slices)
	typeIdx     int // 0=Job, 1=JobSet
	queueIdx    int
	sizeIdx     int
	priorityIdx int // 0=none, 1..N = priorityClasses[i-1]

	// Dynamic option lists updated from snapshot on each refresh
	queues          []watcher.LocalQueueSnapshot // sorted by namespace/name
	priorityClasses []watcher.WorkloadPriorityClassSnapshot // sorted by name

	// Text inputs
	durationInput textinput.Model
	replicasInput textinput.Model
	cpuInput      textinput.Model
	memInput      textinput.Model
	gpuInput      textinput.Model

	// Navigation
	focusedField int

	// Submission context
	kubeconfigPath string

	// Error shown below the form
	submitErr string
}

func newSubmitView(snap watcher.Snapshot, kubeconfigPath string) (submitViewModel, tea.Cmd) {
	dur := textinput.New()
	dur.Prompt = ""
	dur.SetValue("60s")
	dur.CharLimit = 16
	dur.SetWidth(12)

	rep := textinput.New()
	rep.Prompt = ""
	rep.SetValue("2")
	rep.CharLimit = 4
	rep.SetWidth(6)

	cpu := textinput.New()
	cpu.Prompt = ""
	cpu.Placeholder = "e.g. 2"
	cpu.CharLimit = 10
	cpu.SetWidth(12)

	mem := textinput.New()
	mem.Prompt = ""
	mem.Placeholder = "e.g. 8Gi"
	mem.CharLimit = 10
	mem.SetWidth(12)

	gpu := textinput.New()
	gpu.Prompt = ""
	gpu.Placeholder = "e.g. 1"
	gpu.CharLimit = 6
	gpu.SetWidth(8)

	m := submitViewModel{
		durationInput:  dur,
		replicasInput:  rep,
		cpuInput:       cpu,
		memInput:       mem,
		gpuInput:       gpu,
		kubeconfigPath: kubeconfigPath,
	}
	m.refreshOptions(snap)
	cmd := m.applyFocus()
	return m, cmd
}

// refreshOptions rebuilds the queue and priority-class lists from the snapshot.
// Called on open and on each snapshot update while the dialog is open.
func (m *submitViewModel) refreshOptions(snap watcher.Snapshot) {
	// Rebuild queue list.
	queues := make([]watcher.LocalQueueSnapshot, 0, len(snap.LocalQueues))
	for _, lq := range snap.LocalQueues {
		queues = append(queues, lq)
	}
	sort.Slice(queues, func(i, j int) bool {
		ki := queues[i].Namespace + "/" + queues[i].Name
		kj := queues[j].Namespace + "/" + queues[j].Name
		return ki < kj
	})
	m.queues = queues
	if m.queueIdx >= len(m.queues) {
		m.queueIdx = max(0, len(m.queues)-1)
	}

	// Rebuild priority-class list.
	pcs := make([]watcher.WorkloadPriorityClassSnapshot, 0, len(snap.PriorityClasses))
	for _, pc := range snap.PriorityClasses {
		pcs = append(pcs, pc)
	}
	sort.Slice(pcs, func(i, j int) bool { return pcs[i].Name < pcs[j].Name })
	m.priorityClasses = pcs
	if m.priorityIdx > len(m.priorityClasses) {
		m.priorityIdx = 0
	}
}

// visibleFields returns the ordered list of field indices currently in the tab order.
func (m *submitViewModel) visibleFields() []int {
	fields := []int{fieldType, fieldQueue, fieldSize}
	if m.sizeIdx == sizeCustom {
		fields = append(fields, fieldCPU, fieldMem, fieldGPU)
	}
	if m.typeIdx == 1 { // JobSet
		fields = append(fields, fieldReplicas)
	}
	fields = append(fields, fieldDuration, fieldPriority, fieldSubmit)
	return fields
}

func (m *submitViewModel) tabNext() tea.Cmd {
	fields := m.visibleFields()
	for i, f := range fields {
		if f == m.focusedField {
			if i+1 < len(fields) {
				m.focusedField = fields[i+1]
			}
			return m.applyFocus()
		}
	}
	m.focusedField = fields[0]
	return m.applyFocus()
}

func (m *submitViewModel) tabPrev() tea.Cmd {
	fields := m.visibleFields()
	for i, f := range fields {
		if f == m.focusedField {
			if i > 0 {
				m.focusedField = fields[i-1]
			}
			return m.applyFocus()
		}
	}
	m.focusedField = fields[len(fields)-1]
	return m.applyFocus()
}

// ensureFieldVisible ensures focusedField is still in the visible field list.
// Called whenever typeIdx or sizeIdx changes.
func (m *submitViewModel) ensureFieldVisible() tea.Cmd {
	for _, f := range m.visibleFields() {
		if f == m.focusedField {
			return nil
		}
	}
	m.focusedField = fieldType
	return m.applyFocus()
}

func (m *submitViewModel) applyFocus() tea.Cmd {
	m.durationInput.Blur()
	m.replicasInput.Blur()
	m.cpuInput.Blur()
	m.memInput.Blur()
	m.gpuInput.Blur()

	switch m.focusedField {
	case fieldDuration:
		return m.durationInput.Focus()
	case fieldReplicas:
		return m.replicasInput.Focus()
	case fieldCPU:
		return m.cpuInput.Focus()
	case fieldMem:
		return m.memInput.Focus()
	case fieldGPU:
		return m.gpuInput.Focus()
	}
	return nil
}

// update handles key events for the dialog. Returns a tea.Cmd when a
// submission is initiated.
func (m *submitViewModel) update(msg tea.KeyPressMsg, keys keyMap) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Tab):
		return m.tabNext()
	case key.Matches(msg, keys.ShiftTab):
		return m.tabPrev()
	}

	// Text inputs handle their own keys when focused.
	switch m.focusedField {
	case fieldDuration:
		var cmd tea.Cmd
		m.durationInput, cmd = m.durationInput.Update(msg)
		return cmd
	case fieldReplicas:
		var cmd tea.Cmd
		m.replicasInput, cmd = m.replicasInput.Update(msg)
		return cmd
	case fieldCPU:
		var cmd tea.Cmd
		m.cpuInput, cmd = m.cpuInput.Update(msg)
		return cmd
	case fieldMem:
		var cmd tea.Cmd
		m.memInput, cmd = m.memInput.Update(msg)
		return cmd
	case fieldGPU:
		var cmd tea.Cmd
		m.gpuInput, cmd = m.gpuInput.Update(msg)
		return cmd
	}

	// j/k navigate between fields on option selectors and the submit button.
	// left/right (h/l) cycle within the current option selector.
	switch {
	case key.Matches(msg, keys.Up):
		return m.tabPrev()
	case key.Matches(msg, keys.Down):
		return m.tabNext()
	}

	switch m.focusedField {
	case fieldType:
		switch {
		case key.Matches(msg, keys.Left):
			if m.typeIdx > 0 {
				m.typeIdx--
				return m.ensureFieldVisible()
			}
		case key.Matches(msg, keys.Right):
			if m.typeIdx < 1 {
				m.typeIdx++
				return m.ensureFieldVisible()
			}
		}

	case fieldQueue:
		switch {
		case key.Matches(msg, keys.Left):
			if m.queueIdx > 0 {
				m.queueIdx--
			}
		case key.Matches(msg, keys.Right):
			if m.queueIdx < len(m.queues)-1 {
				m.queueIdx++
			}
		}

	case fieldSize:
		switch {
		case key.Matches(msg, keys.Left):
			if m.sizeIdx > 0 {
				m.sizeIdx--
				return m.ensureFieldVisible()
			}
		case key.Matches(msg, keys.Right):
			if m.sizeIdx < len(sizePresets)-1 {
				m.sizeIdx++
				return m.ensureFieldVisible()
			}
		}

	case fieldPriority:
		switch {
		case key.Matches(msg, keys.Left):
			if m.priorityIdx > 0 {
				m.priorityIdx--
			}
		case key.Matches(msg, keys.Right):
			if m.priorityIdx < len(m.priorityClasses) {
				m.priorityIdx++
			}
		}

	case fieldSubmit:
		if key.Matches(msg, keys.Enter) {
			return m.submitCmd()
		}
	}

	// Enter on non-submit fields advances to the next field.
	if key.Matches(msg, keys.Enter) && m.focusedField != fieldSubmit {
		return m.tabNext()
	}

	return nil
}

// submitCmd validates inputs and returns a tea.Cmd that performs the submission.
func (m *submitViewModel) submitCmd() tea.Cmd {
	m.submitErr = ""

	if len(m.queues) == 0 {
		m.submitErr = "no LocalQueues available"
		return nil
	}
	lq := m.queues[m.queueIdx]

	var cpu, mem, gpu resource.Quantity
	if m.sizeIdx == sizeCustom {
		var err error
		cpuStr := strings.TrimSpace(m.cpuInput.Value())
		memStr := strings.TrimSpace(m.memInput.Value())
		gpuStr := strings.TrimSpace(m.gpuInput.Value())
		if cpuStr != "" {
			cpu, err = resource.ParseQuantity(cpuStr)
			if err != nil {
				m.submitErr = "invalid CPU: " + err.Error()
				return nil
			}
		}
		if memStr != "" {
			mem, err = resource.ParseQuantity(memStr)
			if err != nil {
				m.submitErr = "invalid memory: " + err.Error()
				return nil
			}
		}
		if gpuStr != "" {
			gpu, err = resource.ParseQuantity(gpuStr)
			if err != nil {
				m.submitErr = "invalid GPU: " + err.Error()
				return nil
			}
		}
	} else {
		preset := sizePresets[m.sizeIdx]
		cpu = preset.cpu.DeepCopy()
		mem = preset.mem.DeepCopy()
		gpu = preset.gpu.DeepCopy()
	}

	var replicas int32 = 2
	if m.typeIdx == 1 {
		if v, err := strconv.Atoi(strings.TrimSpace(m.replicasInput.Value())); err == nil && v > 0 {
			replicas = int32(v)
		}
	}

	var priorityClass string
	if m.priorityIdx > 0 && m.priorityIdx-1 < len(m.priorityClasses) {
		priorityClass = m.priorityClasses[m.priorityIdx-1].Name
	}

	params := workload.SubmitParams{
		Namespace:     lq.Namespace,
		Queue:         lq.Name,
		PriorityClass: priorityClass,
		CPU:           cpu,
		Memory:        mem,
		GPU:           gpu,
		Replicas:      replicas,
		Duration:      strings.TrimSpace(m.durationInput.Value()),
	}

	isJobSet := m.typeIdx == 1
	kubeconfigPath := m.kubeconfigPath

	return func() tea.Msg {
		client, err := workload.NewWorkloadClient(kubeconfigPath)
		if err != nil {
			return submitResultMsg{err: fmt.Errorf("create client: %w", err)}
		}
		if isJobSet {
			o, g := workload.BuildSubmitJobSet(params)
			err = client.Create(context.Background(), g, o)
		} else {
			o, g := workload.BuildSubmitJob(params)
			err = client.Create(context.Background(), g, o)
		}
		return submitResultMsg{err: err}
	}
}

// --- Rendering ---------------------------------------------------------------

var (
	styleSubmitBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorMuted).
				Padding(0, 2)

	styleSubmitTitle   = lipgloss.NewStyle().Foreground(colorBright).Bold(true)
	styleFieldLabel    = lipgloss.NewStyle().Foreground(colorMuted).Width(12)
	styleOptionActive   = lipgloss.NewStyle().Background(lipgloss.Color("27")).Foreground(colorBright).Bold(true)
	styleOptionInactive = lipgloss.NewStyle().Foreground(colorMuted)
	styleOptionFocused  = lipgloss.NewStyle().Foreground(colorNormal)
	styleSubmitBtn     = lipgloss.NewStyle().Foreground(colorBright).Bold(true)
	styleSubmitError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func (m *submitViewModel) view(termWidth, termHeight int) string {
	const innerWidth = 52

	var sb strings.Builder

	sb.WriteString(styleSubmitTitle.Render("Submit Workload"))
	sb.WriteString("\n\n")

	// Type
	sb.WriteString(m.renderFieldLabel("Type", fieldType))
	sb.WriteString(m.renderOptions(fieldType, []string{"Job", "JobSet"}, m.typeIdx))
	sb.WriteString("\n")

	// Queue
	sb.WriteString(m.renderFieldLabel("Queue", fieldQueue))
	if len(m.queues) == 0 {
		sb.WriteString(styleSubmitError.Render("no queues found"))
	} else {
		labels := make([]string, len(m.queues))
		for i, lq := range m.queues {
			if lq.Namespace == "default" {
				labels[i] = lq.Name
			} else {
				labels[i] = lq.Namespace + "/" + lq.Name
			}
		}
		sb.WriteString(m.renderOptions(fieldQueue, labels, m.queueIdx))
	}
	sb.WriteString("\n")

	// Size
	sb.WriteString(m.renderFieldLabel("Size", fieldSize))
	sizeLabels := make([]string, len(sizePresets))
	for i, p := range sizePresets {
		sizeLabels[i] = p.label
	}
	sb.WriteString(m.renderOptions(fieldSize, sizeLabels, m.sizeIdx))
	sb.WriteString("\n")

	// Custom resource inputs (only when Size=Custom)
	if m.sizeIdx == sizeCustom {
		sb.WriteString(m.renderTextInputField("  CPU", fieldCPU, &m.cpuInput))
		sb.WriteString(m.renderTextInputField("  Memory", fieldMem, &m.memInput))
		sb.WriteString(m.renderTextInputField("  GPU", fieldGPU, &m.gpuInput))
	}

	// Replicas (only when Type=JobSet)
	if m.typeIdx == 1 {
		sb.WriteString(m.renderTextInputField("Replicas", fieldReplicas, &m.replicasInput))
	}

	// Duration
	sb.WriteString(m.renderTextInputField("Duration", fieldDuration, &m.durationInput))

	// Priority
	sb.WriteString(m.renderFieldLabel("Priority", fieldPriority))
	if len(m.priorityClasses) == 0 {
		if m.focusedField == fieldPriority {
			sb.WriteString(styleOptionFocused.Render("none"))
		} else {
			sb.WriteString(styleOptionInactive.Render("none"))
		}
	} else {
		pcLabels := make([]string, len(m.priorityClasses)+1)
		pcLabels[0] = "none"
		for i, pc := range m.priorityClasses {
			pcLabels[i+1] = fmt.Sprintf("%s (%d)", pc.Name, pc.Value)
		}
		sb.WriteString(m.renderOptions(fieldPriority, pcLabels, m.priorityIdx))
	}
	sb.WriteString("\n")

	// Submit button
	sb.WriteString("\n")
	submitLabel := "[ Submit ]"
	if m.focusedField == fieldSubmit {
		sb.WriteString(styleSubmitBtn.Render("▶ " + submitLabel))
	} else {
		sb.WriteString(styleOptionInactive.Render("  " + submitLabel))
	}
	sb.WriteString("\n")

	// Error line
	if m.submitErr != "" {
		sb.WriteString("\n")
		sb.WriteString(styleSubmitError.Render("✗ " + m.submitErr))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(stylePickerCurrent.Render("[tab] next  [↑↓←→] choose  [enter] confirm  [esc] cancel"))

	boxed := styleSubmitBorder.Width(innerWidth).Render(sb.String())

	return centerBox(boxed, termWidth, termHeight)
}

func (m *submitViewModel) renderFieldLabel(label string, field int) string {
	style := styleFieldLabel
	if m.focusedField == field {
		style = style.Foreground(colorBright)
	}
	return style.Render(label+":") + " "
}

func (m *submitViewModel) renderOptions(field int, labels []string, selected int) string {
	focused := m.focusedField == field
	var parts []string
	for i, label := range labels {
		if i == selected {
			parts = append(parts, styleOptionActive.Render(" "+label+" "))
		} else if focused {
			parts = append(parts, styleOptionFocused.Render(" "+label+" "))
		} else {
			parts = append(parts, styleOptionInactive.Render(" "+label+" "))
		}
	}
	return strings.Join(parts, " ")
}

func (m *submitViewModel) renderTextInputField(label string, field int, input *textinput.Model) string {
	return m.renderFieldLabel(label, field) + input.View() + "\n"
}

