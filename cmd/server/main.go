package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/k0ngk0ng/claude-sync/internal/service"
)

func main() {
	port := flag.Int("port", 8080, "监听端口")
	dataDir := flag.String("data", "./claude-sync-data", "数据目录")
	token := flag.String("token", "", "认证令牌 (必填)")
	flag.Parse()

	if *token == "" {
		fmt.Println("错误: 必须指定认证令牌 (-token)")
		fmt.Println()
		fmt.Println("用法:")
		fmt.Println("  claude-sync-server -token <your-secret-token> [-port 8080] [-data ./data]")
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  claude-sync-server -token my-secret-123 -port 8080 -data /data/claude-sync")
		os.Exit(1)
	}

	server := service.NewServer(*port, *dataDir, *token)
	if err := server.Start(); err != nil {
		fmt.Printf("服务器错误: %v\n", err)
		os.Exit(1)
	}
}
