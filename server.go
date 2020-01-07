/*
ps -ef | grep go | awk '{print $2}' | xargs kill -9
go run server.go -one=taro,192.168.0.25,root,password -two=jiro,192.168.0.26,root,password -debug
/root/console-demo/console-demo -port=10000 -html=/root/console-demo/www -org=http://192.168.0.100:8080/ -ws=ws://192.168.0.100:8080/one
/root/console-demo/console-demo -port=20000 -html=/root/console-demo/www -org=http://192.168.0.100:8080/ -ws=ws://192.168.0.100:8080/two

http://192.168.0.25:10000/?token=passwd
http://192.168.0.26:20000/?token=passwd

http://127.0.0.1:10000/?token=passwd
http://127.0.0.1:20000/?token=passwd
*/
package main

import (
  "log"
  "os"
  "fmt"
  "bytes"
  "golang.org/x/crypto/ssh"
  "strings"
  "bufio"
  "flag"
  "time"
  "strconv"
  "net/http"
  "github.com/gorilla/websocket"
  "github.com/nsf/termbox-go"

  ui "github.com/gizak/termui/v3"
  "github.com/gizak/termui/v3/widgets"
)

type normaDataStruct struct {
  Percent int    `json:"percent"`
  Command string `json:"command"`
  Answer  string `json:"answer"`
}

type normaDataStructs []*normaDataStruct

var normaData normaDataStructs

var passwd string
var debug bool = false
var playerOne string
var playerTwo string
var playerOneSock string
var playerTwoSock string
var interval int
var playerOneNorma int
var playerTwoNorma int
var winner int

var upgrader = websocket.Upgrader{
  ReadBufferSize:  1024,
  WriteBufferSize: 1024,
}

func main() {
  _playerOne       := flag.String("one","taro,192.168.0.1,root,passwd","[-one=PLAYER ONE SERVER{NAME,IP,ACCOUNT,PASSWORD} ex) taro,192.168.0.1,root,password]")
  _playerTwo       := flag.String("two","jiro,192.168.0.2,root,passwd","[-two=PLAYER TWO SERVER{NAME,IP,ACCOUNT,PASSWORD} ex) jiro,192.168.0.2,root,password]")
  _playerOneSocket := flag.String("oneURL","one","[-oneURL=PLAYER ONE: WEB SOCKET URL]")
  _playerTwoSocket := flag.String("twoURL","two","[-twoURL=PLAYER TWO: WEB SOCKET URL]")
  _interval        := flag.Int("int",1,"[-int=COMMAND CHECK INTERVAL]")
  _normaConfigFile := flag.String("config","./vsterm.conf","[-config=NORMA CONFIG FILE]")
  _debug    := flag.Bool("debug",false,"[-debug=DEBUG MODE]")
  _port     := flag.String("port","8080","[-port=PORT NUMBER]")

  flag.Parse()

  playerOne        = string(*_playerOne)
  playerTwo        = string(*_playerTwo)
  playerOneSock    = string(*_playerOneSocket)
  playerTwoSock    = string(*_playerTwoSocket)
  interval         = int(*_interval)
  normaConfigFile := string(*_normaConfigFile)
  debug            = bool(*_debug)
  port            := string(*_port)

  if debug == true {
    fmt.Println(" ---------")
    fmt.Println(" |Port   | " + port)
    fmt.Println(" |one    | " + playerOne)
    fmt.Println(" |two    | " + playerTwo)
    fmt.Println(" |1 URL  | ", playerOneSock)
    fmt.Println(" |2 URL  | ", playerTwoSock)
     fmt.Printf(" |int    | %d\n", interval)
     fmt.Printf(" |debug  | %t\n",debug)
    fmt.Println(" |config | " + normaConfigFile)
    fmt.Println(" ---------")
  }

  winner = 0
  playerOneNorma = 0
  playerTwoNorma = 0
  readNormaConfig(normaConfigFile)

  // - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = - = -

  err := termbox.Init()
  if err != nil {
    panic(err)
  }

  defer termbox.Close()
  if err := ui.Init(); err != nil {
    log.Fatalf("failed to initialize termui: %v", err)
  }
  defer ui.Close()

  gOne := widgets.NewGauge()
  gOne.Title = strings.Split(playerOne, ",")[0]
  gOne.Percent = 0
  gOne.SetRect(0, 0, 40,3)
  gOne.BarColor = ui.ColorRed
  gOne.BorderStyle.Fg = ui.ColorCyan
  gOne.TitleStyle.Fg = ui.ColorYellow
  gOne.BorderStyle.Bg = ui.ColorBlack
  gOne.TitleStyle.Bg = ui.ColorBlack

  gTwo := widgets.NewGauge()
  gTwo.Title = strings.Split(playerTwo, ",")[0]
  gTwo.Percent = 0
  gTwo.SetRect(40, 0, 80,3)
  gTwo.BarColor = ui.ColorRed
  gTwo.BorderStyle.Fg = ui.ColorCyan
  gTwo.TitleStyle.Fg = ui.ColorYellow
  gTwo.BorderStyle.Bg = ui.ColorBlack
  gTwo.TitleStyle.Bg = ui.ColorBlack

  pOne := widgets.NewParagraph()
  pOne.TextStyle.Fg = ui.ColorBlack
  //pOne.TextStyle.Bg = ui.ColorBlack
  pOne.Text = normaData[0].Command + "\n" + normaData[0].Answer
  pOne.SetRect(0, 2, 40, 24)
  pOne.BorderStyle.Fg = ui.ColorCyan
  pOne.BorderStyle.Bg = ui.ColorBlack
  
  pTwo := widgets.NewParagraph()
  pTwo.TextStyle.Fg = ui.ColorBlack
  //pTwo.TextStyle.Bg = ui.ColorBlack
  pTwo.Text = normaData[0].Command + "\n" + normaData[0].Answer
  pTwo.SetRect(40, 2, 80, 24)
  pTwo.BorderStyle.Fg = ui.ColorCyan
  pTwo.BorderStyle.Bg = ui.ColorBlack

  ui.Render(gOne,gTwo,pOne,pTwo)

  buffOne := ""
  buffTwo := ""

  go func () {
    uiEvents := ui.PollEvents()
    for {
      e := <-uiEvents
      switch e.ID {
      case "q", "<C-c>":
        ui.Close()
        fmt.Println("Exit..")
        os.Exit(0)
      }
    }
  }()

  go func () {
    for {
      loopFlag := false
      backNorma := playerOneNorma
      
      for {
        playerOneNorma = playerOneNorma + 1
        for _, i := range normaData {
          if playerOneNorma == i.Percent {
            result := strings.Index(execssh(strings.Split(playerOne, ",")[1],strings.Split(playerOne, ",")[2],strings.Split(playerOne, ",")[3],i.Command),i.Answer)
            if debug == true {
              fmt.Printf("Percent (%d) Command (%s) Answer (%s)\n", i.Percent, i.Command, i.Answer)
              fmt.Printf("Result [%d]\n", result)
            }

            if result > -1 {
              loopFlag = true
              playerOneNorma = i.Percent
              gOne.Percent = playerOneNorma
              if i.Percent == 100 { winner = 1 }
              ui.Render(gOne)
            }
            time.Sleep(time.Duration(interval) * time.Second)
          }
        }
        if playerOneNorma > 101 {
          break
        }
      }

      if loopFlag == false {
        playerOneNorma = backNorma
      }

      loopFlag = false
      backNorma = playerTwoNorma

      for {
        playerTwoNorma = playerTwoNorma + 1
        for _, i := range normaData {
          if playerTwoNorma == i.Percent {
            result := strings.Index(execssh(strings.Split(playerTwo, ",")[1],strings.Split(playerTwo, ",")[2],strings.Split(playerTwo, ",")[3],i.Command),i.Answer)
            if debug == true {
              fmt.Printf("Percent (%d) Command (%s) Answer (%s)\n", i.Percent, i.Command, i.Answer)
              fmt.Printf("Result [%d]\n", result)
            }

            if result > -1 {
              loopFlag = true
              playerTwoNorma = i.Percent
              gTwo.Percent = playerTwoNorma
              if i.Percent == 100 { winner = 2 } 
              ui.Render(gTwo)
            }
            time.Sleep(time.Duration(interval) * time.Second)
          }
        }
        if playerTwoNorma > 101 {
          break
        }
      }

      if loopFlag == false {
        playerTwoNorma = backNorma
      }

      switch winner {
        case 1:
          pOne.TextStyle.Fg = ui.ColorYellow
          pTwo.TextStyle.Fg = ui.ColorYellow
          pOne.Text = pOne.Text + "\n\nWinner!!\n\n"
          pTwo.Text = pTwo.Text + "\n\nLose..\n\n"
          ui.Render(gOne,gTwo,pOne,pTwo)
          for {
            time.Sleep(time.Duration(interval * 10) * time.Second)
          }
        case 2:
          pOne.TextStyle.Fg = ui.ColorYellow
          pTwo.TextStyle.Fg = ui.ColorYellow
          pOne.Text = pOne.Text + "\n\nLose..\n\n"
          pTwo.Text = pTwo.Text + "\n\nWinner!!\n\n"
          ui.Render(gOne,gTwo,pOne,pTwo)
          for {
            time.Sleep(time.Duration(interval * 10) * time.Second)
          }
      }
    }
  }()

  counterOne := 0

  http.HandleFunc("/" + playerOneSock, func(w http.ResponseWriter, r *http.Request) {
    conn, _ := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity

    for {
      msgType, msg, err := conn.ReadMessage()
      if err != nil {
          return
      }

      cons := strings.Replace(string(msg), "\"", "", -1)
      if strings.Index(cons,"\\r") != -1 {
        cons  = strings.Replace(cons, "\\r", "", -1)
      }
      if strings.Index(cons,"\\n") != -1 {
        cons  = strings.Replace(cons, "\\n", "", -1)
        if counterOne == 0 {
          buffOne = buffOne + "\n"
        }
        counterOne++;
      }

      if strings.Index(cons,"$ ") == -1 && strings.Index(cons,"# ") == -1 {
        buffOne = buffOne + cons
      } else {
        pOne.Text = buffOne + "\n" + cons
        ui.Render(pOne)
        buffOne = cons
        counterOne = 0
      }

      if err = conn.WriteMessage(msgType, msg); err != nil {
          return
      }
    }
  })

  counterTwo := 0

  http.HandleFunc("/" + playerTwoSock, func(w http.ResponseWriter, r *http.Request) {
    conn, _ := upgrader.Upgrade(w, r, nil) // error ignored for sake of simplicity

    for {
      msgType, msg, err := conn.ReadMessage()
      if err != nil {
          return
      }

      cons := strings.Replace(string(msg), "\"", "", -1)
      if strings.Index(cons,"\\r") != -1 {
        cons  = strings.Replace(cons, "\\r", "", -1)
      }
      if strings.Index(cons,"\\n") != -1 {
        cons  = strings.Replace(cons, "\\n", "", -1)
        if counterTwo == 0 {
          buffTwo = buffTwo + "\n"
        }
        counterTwo++;
      }

      if strings.Index(cons,"$ ") == -1 && strings.Index(cons,"# ") == -1 {
        buffTwo = buffTwo + cons
      } else {
        pTwo.Text = buffTwo + cons
        ui.Render(pTwo)
        buffTwo = cons
        counterTwo = 0
      }

      if err = conn.WriteMessage(msgType, msg); err != nil {
          return
      }
    }
  })

  err = http.ListenAndServe(":" + port, nil)
  if err != nil {
      log.Fatal("error starting http server::", err)
      return
  }

}

func execssh(ip,user,passwd,command string) string {
  var stdoutBuf bytes.Buffer

  config := &ssh.ClientConfig{
    User: user,
    Auth: []ssh.AuthMethod{
      ssh.Password(passwd),
      ssh.KeyboardInteractive(SshInteractive),
    },
    HostKeyCallback: ssh.InsecureIgnoreHostKey(),
  }

  conn, err := ssh.Dial("tcp", ip + ":22", config)
  if err != nil {
    log.Println(err)
  }
  defer conn.Close()

  session, err := conn.NewSession()
  if err != nil {
    log.Println(err)
  }
  defer session.Close()
  session.Stdout = &stdoutBuf
  session.Run(command + " 2>&1 | tee")

  if debug == true {
    fmt.Printf("(%s) ssh exec command: %s\n",ip , command)
    fmt.Printf("[%s]\n", stdoutBuf.String())
  }

  conn.Close()
  return stdoutBuf.String()
}

func SshInteractive(user, instruction string, questions []string, echos []bool) (answers []string, err error) {
  answers = make([]string, len(questions))
  // The second parameter is unused
  for n, _ := range questions {
    answers[n] = passwd
  }

  return answers, nil
}

func replaceNormaStructs(n int,m,o string) (r *normaDataStruct) {
  r = new(normaDataStruct)
  r.Percent = n
  r.Command = m
  r.Answer  = o
  return r
}

func readNormaConfig(configFile string) bool {
  if Exists(configFile) == true {
    var fp *os.File
    var err error

    fp, err = os.Open(configFile)
    if err != nil {
      fmt.Println(err)
      return false
    }
    defer fp.Close()

    normaData = nil

    scanner := bufio.NewScanner(fp)
    for scanner.Scan() {
      readData := scanner.Text()
      i, err := strconv.Atoi(strings.Split(readData, ",")[0])
      if err != nil { 
        fmt.Println(err)
        return false
      }
      normaData = append(normaData, replaceNormaStructs(i, strings.Split(readData, ",")[1], strings.Split(readData, ",")[2]))
    }
    return true
  }
  return false
}

func Exists(name string) bool {
  _, err := os.Stat(name)
  return !os.IsNotExist(err)
}
