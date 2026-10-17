package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Aneeshie/zerrcode/internal/client"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not loaded: %v", err)
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}

	var clnt client.LLMClient = client.NewOpenRouterClient(apiKey)

	ctx := context.Background()
	scanner := bufio.NewScanner(os.Stdin)

	history := []client.Message{
		{
			Role:    client.RoleSystem,
			Content: "You are a helpful coding assistant. Keep your answers concise.",
		},
	}

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

		res, err := clnt.Send(ctx, history)
		if err != nil {
			log.Printf("Request failed: %v", err)
			continue
		}

		history = append(history, client.Message{
			Role:      client.RoleAssistant,
			Content:   res.Content,
			ToolCalls: res.ToolCalls,
		})

		fmt.Println(res.Content)

		if len(res.ToolCalls) > 0 {
			fmt.Printf("\nModel requested %d tool call(s):\n", len(res.ToolCalls))

			for _, toolCall := range res.ToolCalls {
				fmt.Printf("- %s (ID: %s, arguments: %s)\n",
					toolCall.Name,
					toolCall.ID,
					toolCall.Arguments,
				)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Input error: %v", err)
	}
}
