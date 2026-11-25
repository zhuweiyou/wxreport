// wxreport 生成微信群 24h 话题报告所需的辅助命令。
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain 是真正的入口，返回进程退出码，便于测试。
func runMain(args []string, stdout, stderr io.Writer) int {
	root, err := findRoot()
	if err != nil {
		fmt.Fprintln(stderr, "错误:", err)
		return 1
	}
	loadLocalEnv(root)

	env := &Env{Root: root, Stdout: stdout, Stderr: stderr, Getenv: os.Getenv}
	cmd := newRootCmd(env)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(stderr, "错误:", err)
		return 1
	}
	return 0
}
