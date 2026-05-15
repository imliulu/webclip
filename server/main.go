package main

import (
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"

	"github.com/gogf/gf/v2/os/gctx"

	"webclip/server/internal/cmd"
)

func main() {
	cmd.Main.Run(gctx.GetInitCtx())
}
