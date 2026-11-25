// wxreport 生成微信群 24h 话题报告所需的辅助命令。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain 是真正的入口，返回进程退出码，便于测试。
func runMain(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stdout)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	}

	root, err := findRoot()
	if err != nil {
		fmt.Fprintln(stderr, "错误:", err)
		return 1
	}
	loadLocalEnv(root)

	cmd, ok := commandByName[args[0]]
	if !ok {
		usage(stdout)
		return 2
	}

	env := &Env{Root: root, Stdout: stdout, Stderr: stderr, Getenv: os.Getenv}
	if err := cmd.run(context.Background(), env, args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		var fe *flagError
		if errors.As(err, &fe) {
			// flag 已经打印过错误和用法，保持旧版退出码 2。
			return 2
		}
		fmt.Fprintln(stderr, "错误:", err)
		return 1
	}
	return 0
}
