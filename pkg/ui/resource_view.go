package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/malagant/fluxcli/internal/config"
	"github.com/malagant/fluxcli/pkg/k8s"
)

// ResourceView displays FluxCD resources in a table
type ResourceView struct {
	config       *config.Config
	table        table.Model
	resources    []k8s.Resource
	resourceType k8s.ResourceType
	width        int
	height       int
}

// NewResourceView creates a new resource view
func NewResourceView(cfg *config.Config) *ResourceView {
	columns := []table.Column{
		{Title: "Name", Width: cfg.UI.ColumnsName},
		{Title: "Ready", Width: 8},
		{Title: "Status", Width: cfg.UI.ColumnsStatus},
		{Title: "Age", Width: 10},
		{Title: "Message", Width: 40},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return &ResourceView{
		config:       cfg,
		table:        t,
		resourceType: k8s.ResourceTypeGitRepository,
	}
}

// Init initializes the resource view
func (v *ResourceView) Init() tea.Cmd {
	return nil
}

// Update handles messages for the resource view
func (v *ResourceView) Update(msg tea.Msg) (*ResourceView, tea.Cmd) {
	var cmd tea.Cmd
	
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle arrow keys by checking Type directly
		switch msg.Type {
		case tea.KeyDown:
			v.table, cmd = v.table.Update(msg)
		case tea.KeyUp:
			v.table, cmd = v.table.Update(msg)
		case tea.KeyLeft:
			v.table, cmd = v.table.Update(msg)
		case tea.KeyRight:
			v.table, cmd = v.table.Update(msg)
		case tea.KeyPgDown:
			v.table, cmd = v.table.Update(msg)
		case tea.KeyPgUp:
			v.table, cmd = v.table.Update(msg)
		case tea.KeyHome:
			if len(v.resources) > 0 {
				v.table.GotoTop()
			}
		case tea.KeyEnd:
			if len(v.resources) > 0 {
				v.table.GotoBottom()
			}
		case tea.KeyEnter, tea.KeySpace:
			// TODO: Show resource details
			return v, nil
		default:
			// Handle string-based keys
			switch msg.String() {
			// Vertical navigation - j/k for vim users
			case "j":
				v.table, cmd = v.table.Update(tea.KeyMsg{Type: tea.KeyDown})
			case "k":
				v.table, cmd = v.table.Update(tea.KeyMsg{Type: tea.KeyUp})
			
			// Page navigation with vim-style shortcuts
			case "ctrl+d":
				v.table, cmd = v.table.Update(tea.KeyMsg{Type: tea.KeyPgDown})
			case "ctrl+u":
				v.table, cmd = v.table.Update(tea.KeyMsg{Type: tea.KeyPgUp})
			
			// Vim-style navigation
			case "g":
				// Go to top
				if len(v.resources) > 0 {
					v.table.GotoTop()
				}
			case "G":
				// Go to bottom
				if len(v.resources) > 0 {
					v.table.GotoBottom()
				}
			case "H":
				// Go to top of visible area
				if len(v.resources) > 0 {
					v.table.GotoTop()
				}
			case "M":
				// Go to middle of visible area
				if len(v.resources) > 0 {
					middle := len(v.resources) / 2
					for i := 0; i < middle; i++ {
						v.table, _ = v.table.Update(tea.KeyMsg{Type: tea.KeyDown})
					}
				}
			case "L":
				// Go to bottom of visible area
				if len(v.resources) > 0 {
					v.table.GotoBottom()
				}
			}
		}
	}
	
	return v, cmd
}

// View renders the resource view
func (v *ResourceView) View() string {
	if len(v.resources) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Render(fmt.Sprintf("No %s resources found", v.resourceType))
		return emptyMsg
	}
	
	return v.table.View()
}

// SetResources sets the resources to display
func (v *ResourceView) SetResources(resources []k8s.Resource) {
	v.resources = resources
	v.updateTableColumns()
	v.updateTable()
}

// SetResourceType sets the current resource type
func (v *ResourceView) SetResourceType(resourceType k8s.ResourceType) {
	v.resourceType = resourceType
	v.updateTableColumns()
	v.updateTable()
}

// SetSize sets the view dimensions
func (v *ResourceView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.table.SetHeight(height - 2) // Reserve space for borders
	v.updateTableColumns()
}

// updateTable updates the table with current resources
func (v *ResourceView) updateTable() {
	rows := make([]table.Row, 0, len(v.resources))
	
	for _, resource := range v.resources {
		row := v.createTableRow(resource)
		rows = append(rows, row)
	}
	
	v.table.SetRows(rows)
}

// createTableRow creates a table row for a resource
func (v *ResourceView) createTableRow(resource k8s.Resource) table.Row {
	// Format name with namespace if shown
	name := resource.Name
	if v.config.UI.ShowNamespace && resource.Namespace != "" {
		name = fmt.Sprintf("%s/%s", resource.Namespace, resource.Name)
	}
	
	// Format ready status (plain text)
	ready := "False"
	if resource.Ready {
		ready = "True"
	}
	
	// Format status (plain text)
	status := resource.Status
	if status == "" {
		status = "Unknown"
	}
	if resource.Suspended {
		status = "Suspended"
	}
	
	// Truncate status if too long
	if len(status) > 12 {
		status = status[:9] + "…"
	}
	
	// Format age (plain text)
	age := formatAge(resource.Age)
	
	// Format message (truncate if too long)
	message := resource.Message
	if len(message) > 35 {
		message = message[:32] + "…"
	}

	// Resource-specific columns
	switch v.resourceType {
	case k8s.ResourceTypeGitRepository, k8s.ResourceTypeHelmRepository:
		return table.Row{name, ready, status, age, message, resource.URL}
	case k8s.ResourceTypeKustomization:
		source := resource.Source
		if resource.Path != "" {
			source = fmt.Sprintf("%s/%s", source, resource.Path)
		}
		return table.Row{name, ready, status, age, message, source}
	case k8s.ResourceTypeHelmRelease:
		chart := resource.Chart
		if resource.Version != "" {
			chart = fmt.Sprintf("%s:%s", chart, resource.Version)
		}
		return table.Row{name, ready, status, age, message, chart}
	default:
		return table.Row{name, ready, status, age, message}
	}
}

// updateTableColumns updates table columns based on resource type and width
func (v *ResourceView) updateTableColumns() {
	baseColumns := []table.Column{
		{Title: "Name", Width: v.config.UI.ColumnsName},
		{Title: "Ready", Width: 8},
		{Title: "Status", Width: v.config.UI.ColumnsStatus},
		{Title: "Age", Width: 10},
		{Title: "Message", Width: 35},
	}

	// Add resource-specific columns
	switch v.resourceType {
	case k8s.ResourceTypeGitRepository:
		baseColumns = append(baseColumns, table.Column{Title: "URL", Width: 40})
	case k8s.ResourceTypeHelmRepository:
		baseColumns = append(baseColumns, table.Column{Title: "URL", Width: 40})
	case k8s.ResourceTypeKustomization:
		baseColumns = append(baseColumns, table.Column{Title: "Source/Path", Width: 30})
	case k8s.ResourceTypeHelmRelease:
		baseColumns = append(baseColumns, table.Column{Title: "Chart", Width: 25})
	}

	// Adjust column widths based on available space
	if v.width > 0 {
		totalFixedWidth := 0
		flexColumns := 0
		
		for _, col := range baseColumns {
			if col.Title == "Message" || col.Title == "URL" || col.Title == "Source/Path" {
				flexColumns++
			} else {
				totalFixedWidth += col.Width
			}
		}
		
		if flexColumns > 0 {
			availableWidth := v.width - totalFixedWidth - 10 // Reserve space for borders/padding
			flexWidth := availableWidth / flexColumns
			
			if flexWidth > 20 { // Minimum width
				for _, col := range baseColumns {
					if col.Title == "Message" || col.Title == "URL" || col.Title == "Source/Path" {
						col.Width = flexWidth
					}
				}
			}
		}
	}

	v.table.SetColumns(baseColumns)
}

// GetSelectedResource returns the currently selected resource
func (v *ResourceView) GetSelectedResource() *k8s.Resource {
	cursor := v.table.Cursor()
	if cursor >= 0 && cursor < len(v.resources) {
		return &v.resources[cursor]
	}
	return nil
}

// formatAge formats a duration as a human-readable age string
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	} else {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
