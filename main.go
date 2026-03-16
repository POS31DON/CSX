package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	baseStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240"))
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Margin(1, 0)
	
	// Pre-compiled regex to match {variable} patterns securely
	varRegex = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)
)

// Command represents a single tool execution template
type Command struct {
	Tool        string `json:"tool"`
	Description string `json:"description"`
	Phase       string `json:"phase"`
	Command     string `json:"command"`
}

// viewState tracks what screen the UI is currently rendering
type viewState int

const (
	stateTable viewState = iota
	stateForm
	stateExecute
)

// model is the monolithic application state for Bubble Tea
type model struct {
	commands    []Command         // In-memory database
	table       table.Model       // The interactive table component
	search      textinput.Model   // Active fuzzy search input
	filter      string            // Phase category filter
	state       viewState         // Current active UI view
	selectedCmd Command           // The command the user hit enter on

	// Forms variables
	inputs     []textinput.Model // Dynamic array of text inputs for {vars}
	focusIndex int               // Which textinput is currently focused

	// Execution variables
	finalInput textinput.Model // The editable final executed string prompt

	err error // Tracks dynamic application errors
}

// initModel loads the database and sets up the initial application state
func initModel() (model, error) {
	// Resolves the path similarly to bash readlink -f ${BASH_SOURCE[0]}
	exePath, err := os.Executable()
	if err != nil {
		return model{}, fmt.Errorf("could not resolve executable path: %v", err)
	}
	exeDir := filepath.Dir(exePath)
	dbPath := filepath.Join(exeDir, "commands.json")

	// Fallback to local execution for `go run` or direct development
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		dbPath = "commands.json"
	}

	// Native JSON unmarshaling (O(n) speed, entirely in memory)
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return model{}, fmt.Errorf("could not read %s: %v", dbPath, err)
	}

	var cmds []Command
	if err := json.Unmarshal(data, &cmds); err != nil {
		return model{}, fmt.Errorf("could not parse json: %v", err)
	}

	// Build Table Columns
	columns := []table.Column{
		{Title: "Tool", Width: 22},
		{Title: "Description", Width: 42},
		{Title: "Phase", Width: 25},
	}

	// Build Table Rows
	var rows []table.Row
	for _, cmd := range cmds {
		rows = append(rows, table.Row{cmd.Tool, cmd.Description, cmd.Phase})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(15),
	)

	// Style the table
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	// Build Search Input
	ti := textinput.New()
	ti.Placeholder = "Search commands..."
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 50

	m := model{
		commands: cmds,
		table:    t,
		search:   ti,
		filter:   "all",
		state:    stateTable,
	}

	return m, nil
}

// refreshTable filters the internal slice natively using strings.Contains and phase logic
func (m *model) refreshTable() {
	var rows []table.Row
	query := strings.ToLower(m.search.Value())

	for _, cmd := range m.commands {
		// 1. Check hotkey phase filter mapping
		phaseMatch := m.filter == "all" || strings.Contains(strings.ToLower(cmd.Phase), m.filter)
		
		// Map 'network' to match 'ad' or 'privesc' or similar loosely if desired,
		// but using direct fuzzy sub-string map:
		if m.filter == "mobile" {
			phaseMatch = strings.Contains(strings.ToLower(cmd.Phase), "mobile") || strings.Contains(strings.ToLower(cmd.Phase), "thickclient")
		} else if m.filter == "web" {
			phaseMatch = strings.Contains(strings.ToLower(cmd.Phase), "web") || strings.Contains(strings.ToLower(cmd.Phase), "http") || strings.Contains(strings.ToLower(cmd.Phase), "fuzz")
		}

		if !phaseMatch {
			continue
		}

		// 2. Fuzzy Text Query against tool name and description
		textMatch := query == "" ||
			strings.Contains(strings.ToLower(cmd.Tool), query) ||
			strings.Contains(strings.ToLower(cmd.Description), query)

		if textMatch {
			rows = append(rows, table.Row{cmd.Tool, cmd.Description, cmd.Phase})
		}
	}

	m.table.SetRows(rows)
	m.table.SetCursor(0)
}

// Init implements tea.Model
func (m model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model and handles all message and keystroke events
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Global Quit Keys
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "ctrl+c", "ctrl+q", "esc":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	var cmds []tea.Cmd

	// State-Based Routing
	switch m.state {
	case stateTable:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "enter":
				// Attempt to get the selected raw command
				row := m.table.SelectedRow()
				if row == nil {
					return m, nil
				}
				
				// Re-associate the visible row with the full Command struct
				var selected Command
				for _, c := range m.commands {
					if c.Tool == row[0] && c.Description == row[1] {
						selected = c
						break
					}
				}
				m.selectedCmd = selected

				// Extract $\{variables\} dynamically via Posix regex rules
				vars := varRegex.FindAllStringSubmatch(selected.Command, -1)
				
				// Deduplicate and filter variables
				var uniqueVars []string
				seen := make(map[string]bool)
				for _, match := range vars {
					if len(match) > 1 && !seen[match[1]] {
						uniqueVars = append(uniqueVars, match[1])
						seen[match[1]] = true
					}
				}

				if len(uniqueVars) > 0 {
					m.inputs = make([]textinput.Model, len(uniqueVars))
					for i, v := range uniqueVars {
						ti := textinput.New()
						ti.Placeholder = v
						ti.Prompt = v + ": "
						ti.CharLimit = 256
						ti.Width = 30
						if i == 0 {
							ti.Focus()
						}
						m.inputs[i] = ti
					}
					m.focusIndex = 0
					m.state = stateForm
				} else {
					// Prepare execution UI directly
					m.finalInput = textinput.New()
					m.finalInput.SetValue(m.selectedCmd.Command)
					m.finalInput.CharLimit = 500
					m.finalInput.Width = 100
					m.finalInput.Focus()
					m.state = stateExecute
				}
				
				return m, textinput.Blink
			// Phase Filters
			case "ctrl+a":
				m.filter = "all"
				m.refreshTable()
				return m, nil
			case "ctrl+r":
				m.filter = "recon"
				m.refreshTable()
				return m, nil
			case "ctrl+w":
				m.filter = "web"
				m.refreshTable()
				return m, nil
			case "ctrl+p":
				m.filter = "privesc"
				m.refreshTable()
				return m, nil
			case "ctrl+c": // using ctrl+d for cloud to avoid ctrl+c exit
				m.filter = "cloud"
				m.refreshTable()
				return m, nil
			case "ctrl+t": // thick/mobile
				m.filter = "mobile"
				m.refreshTable()
				return m, nil
			case "ctrl+l":
				m.filter = "linux"
				m.refreshTable()
				return m, nil
			}
		}

		// Filter the table on keystroke
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			// Bypass the search input natively entirely if navigating up/down
			if keyMsg.String() == "up" || keyMsg.String() == "down" {
				m.table, cmd = m.table.Update(msg)
				cmds = append(cmds, cmd)
			} else {
				m.table, cmd = m.table.Update(msg)
				cmds = append(cmds, cmd)

				m.search, cmd = m.search.Update(msg)
				cmds = append(cmds, cmd)

				m.refreshTable()
			}
		} else {
			m.table, cmd = m.table.Update(msg)
			cmds = append(cmds, cmd)

			m.search, cmd = m.search.Update(msg)
			cmds = append(cmds, cmd)
		}

		return m, tea.Batch(cmds...)
	case stateForm:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "enter":
				if m.focusIndex == len(m.inputs)-1 {
					// All vars answered, build the final string string
					finalStr := m.selectedCmd.Command
					for _, ti := range m.inputs {
						// ti.Placeholder holds the original "{var}" inner text 
						toReplace := "{" + ti.Placeholder + "}"
						finalStr = strings.ReplaceAll(finalStr, toReplace, ti.Value())
					}
					
					// Prep execution input
					m.finalInput = textinput.New()
					m.finalInput.SetValue(finalStr)
					m.finalInput.CharLimit = 500
					m.finalInput.Width = 100
					m.finalInput.Focus()
					
					m.state = stateExecute
					return m, textinput.Blink
				}

				// Cycle to the next textinput prompt
				m.focusIndex++
				cmdsMsg := make([]tea.Cmd, len(m.inputs))
				for i := 0; i < len(m.inputs); i++ {
					if i == m.focusIndex {
						cmdsMsg[i] = m.inputs[i].Focus()
					} else {
						m.inputs[i].Blur()
					}
				}
				return m, tea.Batch(cmdsMsg...)

			case "up", "shift+tab":
				if m.focusIndex > 0 {
					m.focusIndex--
					cmdsMsg := make([]tea.Cmd, len(m.inputs))
					for i := 0; i < len(m.inputs); i++ {
						if i == m.focusIndex {
							cmdsMsg[i] = m.inputs[i].Focus()
						} else {
							m.inputs[i].Blur()
						}
					}
					return m, tea.Batch(cmdsMsg...)
				}
			}
		}

		// Handle active typing for the focused input safely
		var formCmd tea.Cmd
		m.inputs[m.focusIndex], formCmd = m.inputs[m.focusIndex].Update(msg)
		return m, formCmd

	case stateExecute:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "enter":
				// Confirm final string and signal tea.Quit
				return m, tea.Quit
			}
		}
		var executeCmd tea.Cmd
		m.finalInput, executeCmd = m.finalInput.Update(msg)
		return m, executeCmd
	}

	return m, nil
}

// View implements tea.Model and draws the String representation to the terminal
func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\nError: %v\n\n", m.err)
	}

	switch m.state {
	case stateTable:
		// Attempt to get the selected raw command for the preview
		var previewCmd string
		row := m.table.SelectedRow()
		if row != nil {
			for _, c := range m.commands {
				if c.Tool == row[0] && c.Description == row[1] {
					previewCmd = c.Command
					break
				}
			}
		}

		ui := fmt.Sprintf(
			"CSX - Interactive Security Commands\n\n%s\n\n%s\n\nCommand:\n=== \n%s\n\n%s",
			m.search.View(),
			baseStyle.Render(m.table.View()),
			lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(previewCmd),
			helpStyle.Render("↑/↓: navigate • enter: select • ctrl-[a|r|w|p|c|t|l]: filters • esc: quit"),
		)
		return ui
	case stateForm:
		var b strings.Builder
		b.WriteString("Please provide values for the command parameters:\n\n")

		for i := range m.inputs {
			b.WriteString(m.inputs[i].View())			
			b.WriteRune('\n')
		}

		b.WriteString("\n" + helpStyle.Render("enter: confirm • esc: quit"))
		return b.String()
	case stateExecute:
		return fmt.Sprintf(
			"Press Enter to execute, or modify the command directly:\n\n%s\n\n%s",
			m.finalInput.View(),
			helpStyle.Render("enter: execute • esc: cancel"),
		)
	}

	return "Loading..."
}

func main() {
	m, err := initModel()
	if err != nil {
		fmt.Printf("Startup Error: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m) // Render inline to simulate a small floating window
	
	// `Run()` blocks until tea.Quit is fired
	result, err := p.Run()
	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}

	// Unpack the final model state after GUI close
	finalModel := result.(model)
	
	// If the user hit enter on stateExecute, we have a command string to fire natively!
	cmdStr := finalModel.finalInput.Value()
	if finalModel.state == stateExecute && cmdStr != "" {
		fmt.Printf("\nExecuting: %s\n\n", cmdStr)
		
		// Use bash to securely execute the interpolated string to honor pipes/redirection etc.
		cmd := exec.Command("bash", "-c", cmdStr)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("\nExecution finished with error: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("\nExecution cancelled.")
	}
}
