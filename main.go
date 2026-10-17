package main

import (
	"context"
	"fmt"
	"log"
	"os"

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

	messages := []client.Message{
		{
			Role:    client.RoleSystem,
			Content: "You are a helpful coding assistant. Keep your answers concise.",
		},
		{
			Role:    client.RoleUser,
			Content: "Explain what a goroutine is in Go in 3 sentences.",
		},
	}

	res, err := clnt.Send(ctx, messages)
	if err != nil {
		log.Fatalf("Failed to send request: %v", err)
	}

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
