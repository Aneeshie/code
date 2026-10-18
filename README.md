# zerrcode

`zerrcode` is an interactive, command-line coding assistant written in Go. It uses an LLM (via [OpenRouter](https://openrouter.ai)) to help you read, write, list, and delete files in your workspace through natural conversation.

## Features

- **Conversational coding agent** – chat with an LLM that can reason about your codebase
- **Tool use** – the agent can call file-system tools to inspect and modify files on your behalf
- **Streaming responses** – LLM output is streamed to the terminal in real time
- **Conversation history** – the agent maintains context across your session
- **Path sandboxing** – all file operations are restricted to the workspace directory to prevent path traversal

## Built-in Tools

| Tool | Description |
|------|-------------|
| `read_file` | Read the contents of a file within the workspace |
| `list_files` | List files and directories in a workspace directory |
| `write_file` | Create or overwrite a file with given content |
| `delete_file` | Delete a file within the workspace |

## Requirements

- Go 1.27+
- An [OpenRouter](https://openrouter.ai) API key

## Installation

```bash
git clone https://github.com/Aneeshie/zerrcode.git
cd zerrcode
go mod tidy
go build -o zerrcode .
```

## Usage

1. Create a `.env` file in the project root with your OpenRouter API key:

```env
OPENROUTER_API_KEY=your_openrouter_api_key_here
```

2. Run the program:

```bash
go run main.go
# or
./zerrcode
```

3. Interact at the prompt:

```
zerrcode > list the files in the current directory
zerrcode > read main.go and explain what it does
zerrcode > write a new file called hello.go that prints "Hello, World"
zerrcode > exit
```

Type `exit` (or press `Ctrl+D`) to quit.

## Project Structure

```
zerrcode/
├── main.go                 # CLI entry point and REPL loop
├── go.mod                  # Go module definition
├── go.sum
├── .env                    # Local environment variables (OPENROUTER_API_KEY)
├── .gitignore
├── LICENSE
├── README.md
├── internal/
│   ├── agent/
│   │   └── agent.go        # Agent loop: LLM calls + tool execution
│   ├── client/
│   │   ├── client.go       # LLMClient interface, Message, ToolCall types
│   │   └── openrouter-client.go  # OpenRouter SDK implementation
│   └── tools/
│       ├── tool.go         # Tool interface
│       ├── registry.go     # Tool registry (register/get/execute)
│       ├── path.go         # Workspace path traversal protection
│       ├── read_file.go    # read_file tool
│       ├── list_files.go   # list_files tool
│       ├── write_file.go   # write_file tool
│       └── delete_file.go  # delete_file tool
```

## Configuration

The default model is `nvidia/nemotron-3.5-super-vl:free`. You can change it in `internal/client/openrouter-client.go` by modifying the `model` variable in `convertToChatRequest`.

## License

MIT License. See [LICENSE](LICENSE) for details.