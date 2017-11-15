package main

import (
    "fmt"
    "os/exec"
    "log"
    //"github.com/davecgh/go-spew/spew"
    "strings"
    "os"
    "encoding/json"
    "github.com/jessevdk/go-flags"
    "bufio"
    "regexp"
    "io"
    "net"
)

type Options struct {
    ZabbixAgentConfig string `short:"c" long:"zabbix-agent-config" description:"Path to zabbix_agentd.conf" default:"/etc/zabbix/zabbix_agentd.conf"`
    HostName          string `short:"H" long:"hostname"            description:"Hostname in Zabbix"         default:"from zabbix_agentd.conf"`
    ZabbixServer    []string `short:"z" long:"zabbix-server"       description:"Zabbix server host"         default:"from zabbix_agentd.conf"`
    DriveList       []string `short:"d" long:"dev"                 description:"Device"                     default:"all"`
    Logfile           string `short:"l" long:"log-file"            description:"Logfile"                    default:"stdout"`
    Verbose           bool   `short:"v" long:"verbose"             description:"Verbose mode"`
    PrintOnly         bool   `short:"p" long:"print-only"          description:"No Zabbix Send (print only)"`
    PartSize          int    `short:"s" long:"part-size"           description:"Maximum number of items at a time" default:"200"`
    Version           bool   `short:"V" long:"version"             description:"Show version"`
}

type discoveryType struct {
    Device   string `json:"{#HARDDISK}"`
}

type senderOutput struct {
    Data []discoveryType `json:"data"`
}

var zaConfig map[string]string
var opts Options

var Version string

func init() {
    //_ = spew.Sdump("")

    if _, err := flags.Parse(&opts); err != nil {
        if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
            os.Exit(0)
        } else {
            os.Exit(1)
        }
    }

    if (opts.Version == true) {
        fmt.Printf("Version: %s\n", Version)
        os.Exit(0)
    }

    if opts.PartSize > 250 {
        fmt.Println("Zabbix sender will send up to 250 values in one connection.")
        os.Exit(1)
    }

    zaConfig = readZabbixConfig(opts.ZabbixAgentConfig)

    if opts.ZabbixServer[0] == "from zabbix_agentd.conf" {
        opts.ZabbixServer = strings.Split(zaConfig["Server"], ",")
    }

    if opts.HostName == "from zabbix_agentd.conf" {
        opts.HostName = zaConfig["Hostname"]
    }
}

func main() {
    if len(opts.Logfile) != 0 && opts.Logfile != "stdout" {
        f, err := os.OpenFile(opts.Logfile, os.O_RDWR | os.O_CREATE | os.O_APPEND, 0644)
        if err != nil {
            log.Fatalf("error: %v", err)
        }
        defer f.Close()

        log.SetOutput(f)
    }

    var discoveryHeap []discoveryType
    var itemsHeap []string

    for _, item := range iostat() {
        addFlag := false

        if len(opts.DriveList) > 0 && len(opts.DriveList[0]) > 0 && opts.DriveList[0] != "all" {
            for _, i := range opts.DriveList {
                if i == item["device"] {
                    addFlag = true
                }
            }
        } else {
            addFlag = true
        }

        if addFlag == true {
            device := item["device"]

            discoveryHeap = append(discoveryHeap, discoveryType{
                                    Device: device,
            });

            for k, v := range item {
                if k == "device" {
                    continue
                }

                itemsHeap = append(itemsHeap, fmt.Sprintf(`"%s" iostatmon.metric[%s,%s] "%s"`, zaConfig["Hostname"], device, k, v))
            }
        }
    }

    discovery, err := json.Marshal(senderOutput{
                                       Data: discoveryHeap,
                                   })
    if err != nil {
        log.Fatal(fmt.Sprintf("Can't build discovery JSON: %s\n", err))
    }

    discoveryString := fmt.Sprintf(`"%s" iostatmon.discovery "%s"`,
                                    zaConfig["Hostname"],
                                    strings.Replace(string(discovery), "\"", "\\\"", -1))

    zabbixSend(opts.ZabbixServer, []string{discoveryString})

    for i := len(itemsHeap); i >= 0; i -= opts.PartSize {
        n := i - opts.PartSize
        if (n < 0) {
            n = 0
        }

        zabbixSend(opts.ZabbixServer, itemsHeap[n:i])

        if (n == 0) {
            break
        }
    }
}

func iostat() []map[string]string {
    iostat := strings.Split(execute("/usr/bin/iostat -xky 1 2"), "\n")

    readFlag := false
    var fields []string
    var rv []map[string]string

    for _, line := range iostat[int(len(iostat) / 2):len(iostat)] {
        if len(line) > 0 {
            f := strings.Fields(line)

            if readFlag == true && len(f) == len(fields) {
                item := make(map[string]string)

                for i, v := range f {
                    item[fields[i]] = v
                }

                rv = append(rv, item)
            }

            if f[0] == "Device:" {
                readFlag = true
                f[0] = "device"
                for i, v := range f {
                    if v == "%util" {
                        f[i] = "util"
                    }
                }
                fields = f
            }
        }
    }

    return rv
}

func execute(cmdString string) string {
    parts := strings.Fields(cmdString)

    head := parts[0]
    parts = parts[1:len(parts)]

    out, err := exec.Command(head, parts...).Output()

    if err != nil {
        log.Fatal(err)
    }

    return string(out)
}

func readZabbixConfig(file string) map[string]string {
    rv := make(map[string]string)

    fileh, err := os.Open(file)
    if err != nil {
        log.Fatal(err)
    }

    defer fileh.Close()
    scanner := bufio.NewScanner(fileh)

    for scanner.Scan() {
        line := scanner.Text()

        re := regexp.MustCompile("^\\s*(#|$)")
        next := re.MatchString(line)

        if next {
            continue
        }

        sline := strings.Split(line, "=")
        rv[sline[0]] = sline[1]
    }

    if err := scanner.Err(); err != nil {
        log.Fatal(err)
    }

    return rv
}

func zabbixSend(server []string, data []string) {
    if opts.PrintOnly == true {
        for _, line := range data {
            fmt.Println(line)
        }
    } else {
        for _, zabbixServer := range server {
            conn, err := net.Dial("tcp", zabbixServer + ":10051")
            if err != nil {
                log.Printf("Zabbix server %s not avalible\n", zabbixServer)
                continue
            } else {
                defer conn.Close()
                log.Printf("Sendindg data to %s\n", zabbixServer)
            }

            args := []string{
                "-z",
                zabbixServer,
                "-i",
                "-",
            }

            if opts.Verbose == true {
                args = append(args, "-vv")
            }

            cmd := exec.Command("/usr/bin/zabbix_sender", args...)
            stdin, err := cmd.StdinPipe()

            if err != nil {
                log.Fatal(err)
            }

            go func() {
                defer stdin.Close()
                if opts.Verbose == true {
                    log.Printf("Send data to zabbix server:\n%s", strings.Join(data, "\n"))
                }
                io.WriteString(stdin, strings.Join(data, "\n"))
            }()

            out, err := cmd.CombinedOutput()

            if err != nil {
                log.Printf("Error: %s", err)
            }

            log.Printf("%s", out)
        }
    }
}
