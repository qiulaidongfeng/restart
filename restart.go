package restart

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const Mode = "RESTART_MODE"
const WORKER = "WORKER"
const DirectReturn = 4

var i int

func Run(fn func()) {
	mode := os.Getenv(Mode)
	if mode == WORKER {
		fn()
		return
	}
	runMaster(0)
}

func RunWithDuration(fn func(), d time.Duration) {
	mode := os.Getenv(Mode)
	if mode == WORKER {
		fn()
		return
	}
	runMaster(d)
}

func runMaster(d time.Duration) {
	for {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGABRT,
			syscall.SIGKILL, syscall.SIGTERM)

		name := os.Args[0]
		args := os.Args[1:len(os.Args)]
		cmd := exec.Command(name, args...)
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		cmd.Env = append(os.Environ(), Mode+"="+WORKER)
		if err := cmd.Start(); err != nil {
			log.Fatalln("Cannot start subprocess, exit with err:", err)
		}

		var directExit bool
		subProcessCtx, subProcessCancel := context.WithCancel(context.Background())
		go func() {
			err := cmd.Wait()
			if err != nil {
				if strings.Contains(err.Error(), strconv.Itoa(DirectReturn)) {
					directExit = true
				}
				fmt.Println(err.Error())
			}
			subProcessCancel()
		}()

		stopWorker := make(chan interface{}, 1)

		if d != 0 {
			go func() {
				<-time.After(d)
				stopWorker <- nil
			}()
		}

		select {
		case sig := <-c:
			log.Println("receive signal", sig)
			err := cmd.Process.Signal(sig)
			if err != nil {
				log.Println(err)
				err := cmd.Process.Signal(syscall.SIGBUS)
				if err != nil {
					log.Println(err)
					log.Fatalln("kill subprocess failed, exit")
				}
			}
			<-subProcessCtx.Done()
			os.Exit(0)
		case <-subProcessCtx.Done():
			if directExit {
				log.Println("subprocess exit with code: 4, direct exit")
				os.Exit(4)
			}
			i++
			log.Println(i)
		case <-stopWorker:
			err := cmd.Process.Kill()
			if err != nil {
				log.Fatalln("kill subprocess failed, exit")
			}
		}
	}
}
