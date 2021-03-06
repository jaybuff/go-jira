package main

import (
	"fmt"
	"github.com/Netflix-Skunkworks/go-jira/jira/cli"
	"github.com/docopt/docopt-go"
	"github.com/op/go-logging"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"strings"
)

var log = logging.MustGetLogger("jira")
var format = "%{color}%{time:2006-01-02T15:04:05.000Z07:00} %{level:-5s} [%{shortfile}]%{color:reset} %{message}"

func main() {
	user := os.Getenv("USER")
	home := os.Getenv("HOME")
	usage := fmt.Sprintf(`
Usage:
  jira [-v ...] [-u USER] [-e URI] [-t FILE] (ls|list) ( [-q JQL] | [-p PROJECT] [-c COMPONENT] [-a ASSIGNEE] [-i ISSUETYPE]) 
  jira [-v ...] [-u USER] [-e URI] [-t FILE] view ISSUE
  jira [-v ...] [-u USER] [-e URI] [-t FILE] edit ISSUE [-m COMMENT] [-o KEY=VAL]...
  jira [-v ...] [-u USER] [-e URI] [-t FILE] create [-p PROJECT] [-i ISSUETYPE] [-o KEY=VAL]...
  jira [-v ...] [-u USER] [-e URI] DUPLICATE dups ISSUE
  jira [-v ...] [-u USER] [-e URI] BLOCKER blocks ISSUE
  jira [-v ...] [-u USER] [-e URI] watch ISSUE [-w WATCHER]
  jira [-v ...] [-u USER] [-e URI] (trans|transition) TRANSITION ISSUE [-m COMMENT]
  jira [-v ...] [-u USER] [-e URI] ack ISSUE [-m COMMENT]
  jira [-v ...] [-u USER] [-e URI] close ISSUE [-m COMMENT]
  jira [-v ...] [-u USER] [-e URI] resolve ISSUE [-m COMMENT]
  jira [-v ...] [-u USER] [-e URI] reopen ISSUE [-m COMMENT]
  jira [-v ...] [-u USER] [-e URI] start ISSUE [-m COMMENT]
  jira [-v ...] [-u USER] [-e URI] stop ISSUE [-m COMMENT]
  jira [-v ...] [-u USER] [-e URI] [-t FILE] comment ISSUE [-m COMMENT]
  jira [-v ...] [-u USER] [-e URI] take ISSUE
  jira [-v ...] [-u USER] [-e URI] (assign|give) ISSUE ASSIGNEE
  jira [-v ...] [-u USER] [-e URI] [-t FILE] fields
  jira [-v ...] [-u USER] [-e URI] [-t FILE] issuelinktypes
  jira [-v ...] [-u USER] [-e URI] [-t FILE] transmeta ISSUE
  jira [-v ...] [-u USER] [-e URI] [-t FILE] editmeta ISSUE
  jira [-v ...] [-u USER] [-e URI] [-t FILE] issuetypes [-p PROJECT] 
  jira [-v ...] [-u USER] [-e URI] [-t FILE] createmeta [-p PROJECT] [-i ISSUETYPE] 
  jira [-v ...] [-u USER] [-e URI] [-t FILE] transitions ISSUE
  jira [-v ...] export-templates [-d DIR]
  jira [-v ...] [-u USER] [-e URI] [-t FILE] login
  jira [-v ...] [-u USER] [-e URI] [-t FILE] ISSUE
 
General Options:
  -e --endpoint=URI   URI to use for jira
  -h --help           Show this usage
  -t --template=FILE  Template file to use for output/editing
  -u --user=USER      Username to use for authenticaion (default: %s)
  -v --verbose        Increase output logging
  --version           Show this version

Command Options:
  -a --assignee=USER        Username assigned the issue
  -c --component=COMPONENT  Component to Search for
  -d --directory=DIR        Directory to export templates to (default: %s)
  -i --issuetype=ISSUETYPE  Jira Issue Type (default: Bug)
  -m --comment=COMMENT      Comment message for transition
  -o --override=KEY:VAL     Set custom key/value pairs
  -p --project=PROJECT      Project to Search for
  -q --query=JQL            Jira Query Language expression for the search
  -w --watcher=USER         Watcher to add to issue (default: %s)
`, user, fmt.Sprintf("%s/.jira.d/templates", home), user)

	args, err := docopt.Parse(usage, nil, true, "0.0.1", false, false)
	if err != nil {
		log.Error("Failed to parse options: %s", err)
		os.Exit(1)
	}
	logBackend := logging.NewLogBackend(os.Stderr, "", 0)
	logging.SetBackend(
		logging.NewBackendFormatter(
			logBackend,
			logging.MustStringFormatter(format),
		),
	)
	logging.SetLevel(logging.NOTICE, "")
	if verbose, ok := args["--verbose"]; ok {
		if verbose.(int) > 1 {
			logging.SetLevel(logging.DEBUG, "")
		} else if verbose.(int) > 0 {
			logging.SetLevel(logging.INFO, "")
		}
	}

	log.Info("Args: %v", args)

	opts := make(map[string]string)
	loadConfigs(opts)

	// strip the "--" off the command line options
	// and populate the opts that we pass to the cli ctor
	for key, val := range args {
		if val != nil && strings.HasPrefix(key, "--") {
			opt := key[2:]
			if opt == "override" {
				for _, v := range val.([]string) {
					if strings.Contains(v, "=") {
						kv := strings.SplitN(v, "=", 2)
						opts[kv[0]] = kv[1]
					} else {
						log.Error("Malformed override, expected KEY=VALUE, got %s", v)
						os.Exit(1)
					}
				}
			} else {
				switch v := val.(type) {
				case string:
					opts[opt] = v
				case int:
					opts[opt] = fmt.Sprintf("%d", v)
				}
			}
		}
	}

	// cant use proper [default:x] syntax in docopt
	// because only want to default if the option is not
	// already specified in some .jira.d/config.yml file
	if _, ok := opts["user"]; !ok {
		opts["user"] = user
	}
	if _, ok := opts["issuetype"]; !ok {
		opts["issuetype"] = "Bug"
	}
	if _, ok := opts["directory"]; !ok {
		opts["directory"] = fmt.Sprintf("%s/.jira.d/templates", home)
	}

	if _, ok := opts["endpoint"]; !ok {
		log.Error("endpoint option required.  Either use --endpoint or set a enpoint option in your ~/.jira.d/config.yml file")
		os.Exit(1)
	}

	c := cli.New(opts)

	log.Debug("opts: %s", opts)

	validCommand := func(cmd string) bool {
		if val, ok := args[cmd]; ok && val.(bool) {
			return true
		}
		return false
	}

	validOpt := func(opt string, dflt interface{}) interface{} {
		if val, ok := opts[opt]; ok {
			return val
		}
		if dflt == nil {
			log.Error("Missing required option --%s or \"%s\" property override in the config file", opt, opt)
			os.Exit(1)
		}
		return dflt
	}

	if validCommand("login") {
		err = c.CmdLogin()
	} else if validCommand("fields") {
		err = c.CmdFields()
	} else if validCommand("ls") || validCommand("list") {
		err = c.CmdList()
	} else if validCommand("edit") {
		err = c.CmdEdit(args["ISSUE"].(string))
	} else if validCommand("editmeta") {
		err = c.CmdEditMeta(args["ISSUE"].(string))
	} else if validCommand("transmeta") {
		err = c.CmdTransitionMeta(args["ISSUE"].(string))
	} else if validCommand("issuelinktypes") {
		err = c.CmdIssueLinkTypes()
	} else if validCommand("issuetypes") {
		err = c.CmdIssueTypes(validOpt("project", nil).(string))
	} else if validCommand("createmeta") {
		err = c.CmdCreateMeta(
			validOpt("project", nil).(string),
			validOpt("issuetype", "Bug").(string),
		)
	} else if validCommand("create") {
		err = c.CmdCreate(
			validOpt("project", nil).(string),
			validOpt("issuetype", "Bug").(string),
		)
	} else if validCommand("transitions") {
		err = c.CmdTransitions(args["ISSUE"].(string))
	} else if validCommand("blocks") {
		err = c.CmdBlocks(
			args["BLOCKER"].(string),
			args["ISSUE"].(string),
		)
	} else if validCommand("dups") {
		err = c.CmdDups(
			args["DUPLICATE"].(string),
			args["ISSUE"].(string),
		)
	} else if validCommand("watch") {
		err = c.CmdWatch(
			args["ISSUE"].(string),
			validOpt("watcher", user).(string),
		)
	} else if validCommand("trans") || validCommand("transition") {
		err = c.CmdTransition(
			args["ISSUE"].(string),
			args["TRANSITION"].(string),
		)
	} else if validCommand("close") {
		err = c.CmdTransition(args["ISSUE"].(string), "close")
	} else if validCommand("ack") {
		err = c.CmdTransition(args["ISSUE"].(string), "acknowledge")
	} else if validCommand("reopen") {
		err = c.CmdTransition(args["ISSUE"].(string), "reopen")
	} else if validCommand("resolve") {
		err = c.CmdTransition(args["ISSUE"].(string), "resolve")
	} else if validCommand("start") {
		err = c.CmdTransition(args["ISSUE"].(string), "start")
	} else if validCommand("stop") {
		err = c.CmdTransition(args["ISSUE"].(string), "stop")
	} else if validCommand("comment") {
		err = c.CmdComment(args["ISSUE"].(string))
	} else if validCommand("take") {
		err = c.CmdAssign(args["ISSUE"].(string), user)
	} else if validCommand("export-templates") {
		err = c.CmdExportTemplates()
	} else if validCommand("assign") || validCommand("give") {
		err = c.CmdAssign(
			args["ISSUE"].(string),
			args["ASSIGNEE"].(string),
		)
	} else if val, ok := args["ISSUE"]; ok {
		err = c.CmdView(val.(string))
	}

	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func parseYaml(file string, opts map[string]string) {
	if fh, err := ioutil.ReadFile(file); err == nil {
		log.Debug("Found Config file: %s", file)
		yaml.Unmarshal(fh, &opts)
	}
}

func loadConfigs(opts map[string]string) {
	paths := cli.FindParentPaths(".jira.d/config.yml")
	// prepend
	paths = append([]string{"/etc/jira-cli.yml"}, paths...)

	for _, file := range paths {
		parseYaml(file, opts)
	}
}
