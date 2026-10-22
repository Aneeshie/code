package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Aneeshie/zerrcode/internal/agent"
	"github.com/Aneeshie/zerrcode/internal/client"
	"github.com/Aneeshie/zerrcode/internal/permissions"
	"github.com/Aneeshie/zerrcode/internal/tools"
	"github.com/Aneeshie/zerrcode/internal/tui"
	"github.com/joho/godotenv"
)

func main() {
	useTUI := flag.Bool("tui", false, "Start with interactive Bubble Tea TUI")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not loaded: %v", err)
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}

	var clnt client.LLMClient = client.NewOpenRouterClient(apiKey)

	registry := tools.NewRegistry()

	registry.Register(tools.NewReadFileTool("."))
	registry.Register(tools.NewListFilesTool("."))
	registry.Register(tools.NewWriteFileTool("."))
	registry.Register(tools.NewDeleteFileTool("."))
	registry.Register(tools.NewSearchCodeTool("."))
	registry.Register(tools.NewEditFileTool("."))
	registry.Register(tools.NewGitDiffTool("."))

	codingAgent := agent.NewAgent(clnt, registry)

	history := []client.Message{
		{
			Role:    client.RoleSystem,
			Content: agent.SystemPrompt,
		},
	}

	if *useTUI {
		if err := tui.Run(codingAgent, history); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
		return
	}

	// For standard REPL, allow all changes without interactive TUI prompts
	codingAgent.SetApprover(permissions.AllowAll{})

	ctx := context.Background()
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("zerrcode — type 'exit' to quit.")

	for {
		fmt.Print("\nzerrcode > ")

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		if input == "exit" {
			fmt.Println("Goodbye!")
			break
		}

		if input == "" {
			continue
		}

		history = append(history, client.Message{
			Role:    client.RoleUser,
			Content: input,
		})

		updatedHistory, err := codingAgent.Run(
			ctx,
			history,
			func(chunk string) {
				fmt.Print(chunk)
			},
		)
		if err != nil {
			fmt.Printf("\nRequest failed: %v\n", err)
			continue
		}

		history = updatedHistory
		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Input error: %v", err)
	}
}
