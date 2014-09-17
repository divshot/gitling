package main

import (
  "os"
  "github.com/codegangsta/cli"
  
  "github.com/divshot/gitling/server"
)

func main() {
  app := cli.NewApp()
  app.Name  = "gitling"
  app.Usage = "a smart git HTTP server with dynamic authentication."
  app.Flags = []cli.Flag {
    cli.StringFlag{
      Name: "auth-url, a",
      Value: "",
      Usage: "A URL to which request information will be POST-ed for authentication",
      EnvVar: "GITLING_AUTH_URL",
    },
    cli.StringFlag{
      Name: "port, p",
      Value: "8080",
      Usage: "The port upon which to run the server",
      EnvVar: "PORT",
    },
    cli.StringFlag{
      Name: "root, r",
      Value: "",
      Usage: "The root directory for all git repositories.",
      EnvVar: "PWD",
    },
  }
  app.Action = func(c *cli.Context) {
    config := server.Config{
      ProjectRoot: c.String("root"),
      Port: c.String("port"),
      AuthURL: c.String("auth-url"),
      UploadPack: true,
      ReceivePack: true,
    }
    
    server.Start(config)
  }
  
  app.Run(os.Args)
}