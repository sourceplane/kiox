package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("no capability")
		return
	}
	fmt.Printf("capability=%s args=%s home=%s\n", args[0], strings.Join(args[1:], ","), os.Getenv("KIOX_PROVIDER_HOME"))
}
