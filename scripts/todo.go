package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

type TodoItem struct {
	Text        string
	Priority    string
	Category    string
	Status      string
	DateAdded   time.Time
	DateUpdated time.Time
}

type TodoManager struct {
	filePath string
	items    []TodoItem
}

func NewTodoManager(filePath string) *TodoManager {
	return &TodoManager{
		filePath: filePath,
		items:    []TodoItem{},
	}
}

func (tm *TodoManager) AddTodo(text, priority, category string) {
	item := TodoItem{
		Text:        text,
		Priority:    priority,
		Category:    category,
		Status:      "todo",
		DateAdded:   time.Now(),
		DateUpdated: time.Now(),
	}
	tm.items = append(tm.items, item)
	tm.SaveToFile()
}

func (tm *TodoManager) MarkComplete(index int) {
	if index >= 0 && index < len(tm.items) {
		tm.items[index].Status = "completed"
		tm.items[index].DateUpdated = time.Now()
		tm.SaveToFile()
	}
}

func (tm *TodoManager) MarkInProgress(index int) {
	if index >= 0 && index < len(tm.items) {
		tm.items[index].Status = "in_progress"
		tm.items[index].DateUpdated = time.Now()
		tm.SaveToFile()
	}
}

func (tm *TodoManager) LoadFromFile() error {
	file, err := os.Open(tm.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentSection := ""
	todoPattern := regexp.MustCompile(`^- \[([ x])\] (.+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if strings.HasPrefix(line, "## 🚧 In Progress") {
			currentSection = "in_progress"
			continue
		} else if strings.HasPrefix(line, "## 📋 Todo") {
			currentSection = "todo"
			continue
		} else if strings.HasPrefix(line, "## ✅ Completed") {
			currentSection = "completed"
			continue
		}

		matches := todoPattern.FindStringSubmatch(line)
		if len(matches) == 3 {
			status := "todo"
			if currentSection == "in_progress" {
				status = "in_progress"
			} else if currentSection == "completed" || matches[1] == "x" {
				status = "completed"
			}

			text := matches[2]
			priority, category := tm.extractPriorityAndCategory(text)

			item := TodoItem{
				Text:        text,
				Priority:    priority,
				Category:    category,
				Status:      status,
				DateAdded:   time.Now(), // Default to now if not found
				DateUpdated: time.Now(),
			}
			tm.items = append(tm.items, item)
		}
	}

	return scanner.Err()
}

func (tm *TodoManager) extractPriorityAndCategory(text string) (string, string) {
	priority := "medium"
	category := "general"

	if strings.Contains(text, "🔥") {
		priority = "high"
	} else if strings.Contains(text, "🔵") {
		priority = "low"
	}

	if strings.Contains(text, "🐛") {
		category = "bug"
	} else if strings.Contains(text, "✨") {
		category = "feature"
	} else if strings.Contains(text, "🔧") {
		category = "maintenance"
	} else if strings.Contains(text, "📚") {
		category = "documentation"
	} else if strings.Contains(text, "🧪") {
		category = "testing"
	} else if strings.Contains(text, "🚀") {
		category = "deployment"
	}

	return priority, category
}

func (tm *TodoManager) SaveToFile() error {
	file, err := os.Create(tm.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	// Write header
	fmt.Fprintln(writer, "# AudiModal Todo List")
	fmt.Fprintln(writer, "")

	// Write in progress items
	fmt.Fprintln(writer, "## 🚧 In Progress")
	fmt.Fprintln(writer, "")
	for _, item := range tm.items {
		if item.Status == "in_progress" {
			fmt.Fprintf(writer, "- [ ] %s\n", item.Text)
		}
	}
	fmt.Fprintln(writer, "")

	// Write todo items
	fmt.Fprintln(writer, "## 📋 Todo")
	fmt.Fprintln(writer, "")
	for _, item := range tm.items {
		if item.Status == "todo" {
			fmt.Fprintf(writer, "- [ ] %s\n", item.Text)
		}
	}
	fmt.Fprintln(writer, "")

	// Write completed items
	fmt.Fprintln(writer, "## ✅ Completed")
	fmt.Fprintln(writer, "")
	for _, item := range tm.items {
		if item.Status == "completed" {
			fmt.Fprintf(writer, "- [x] %s\n", item.Text)
		}
	}

	// Write footer with usage instructions
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "---")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "## Quick Reference")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "### Priority Levels")
	fmt.Fprintln(writer, "- 🔥 **High Priority** - Critical tasks that block other work")
	fmt.Fprintln(writer, "- 🔶 **Medium Priority** - Important but not blocking")
	fmt.Fprintln(writer, "- 🔵 **Low Priority** - Nice to have, can be deferred")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "### Categories")
	fmt.Fprintln(writer, "- 🐛 **Bug Fix** - Issues that need resolution")
	fmt.Fprintln(writer, "- ✨ **Feature** - New functionality to implement")
	fmt.Fprintln(writer, "- 🔧 **Maintenance** - Code cleanup, refactoring, updates")
	fmt.Fprintln(writer, "- 📚 **Documentation** - Docs, comments, README updates")
	fmt.Fprintln(writer, "- 🧪 **Testing** - Unit tests, integration tests, test improvements")
	fmt.Fprintln(writer, "- 🚀 **Deployment** - CI/CD, infrastructure, release tasks")

	return nil
}

func (tm *TodoManager) ListTodos() {
	fmt.Println("Todo Items:")
	for i, item := range tm.items {
		status := "[ ]"
		if item.Status == "completed" {
			status = "[x]"
		} else if item.Status == "in_progress" {
			status = "[~]"
		}
		fmt.Printf("%d. %s %s (Priority: %s, Category: %s)\n", 
			i+1, status, item.Text, item.Priority, item.Category)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run todo.go <command> [args...]")
		fmt.Println("Commands:")
		fmt.Println("  add \"text\" [priority] [category]")
		fmt.Println("  list")
		fmt.Println("  complete <index>")
		fmt.Println("  progress <index>")
		return
	}

	todoFile := "../TODO.md"
	tm := NewTodoManager(todoFile)
	tm.LoadFromFile()

	command := os.Args[1]

	switch command {
	case "add":
		if len(os.Args) < 3 {
			log.Fatal("Please provide todo text")
		}
		text := os.Args[2]
		priority := "medium"
		category := "general"
		
		if len(os.Args) > 3 {
			priority = os.Args[3]
		}
		if len(os.Args) > 4 {
			category = os.Args[4]
		}
		
		tm.AddTodo(text, priority, category)
		fmt.Printf("Added todo: %s\n", text)

	case "list":
		tm.ListTodos()

	case "complete":
		if len(os.Args) < 3 {
			log.Fatal("Please provide todo index")
		}
		var index int
		fmt.Sscanf(os.Args[2], "%d", &index)
		tm.MarkComplete(index - 1) // Convert to 0-based index
		fmt.Printf("Marked item %d as complete\n", index)

	case "progress":
		if len(os.Args) < 3 {
			log.Fatal("Please provide todo index")
		}
		var index int
		fmt.Sscanf(os.Args[2], "%d", &index)
		tm.MarkInProgress(index - 1) // Convert to 0-based index
		fmt.Printf("Marked item %d as in progress\n", index)

	default:
		fmt.Printf("Unknown command: %s\n", command)
	}
}