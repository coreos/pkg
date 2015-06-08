package capnslog

import (
	"fmt"
	"log"
	"os"
	"testing"
)

func TestFmt(t *testing.T) {
	fmt.Println("foo")
}

func TestLog(t *testing.T) {
	SetFormatter(NewStringFormatter(os.Stdout))
	log.Println("foo")
}

func TestCapnslogCaptureAtInfo(t *testing.T) {
	MustRepoLogger("log").SetRepoLogLevel(ERROR)
	SetFormatter(NewStringFormatter(os.Stdout))
	log.Println("at error")
	MustRepoLogger("log").SetRepoLogLevel(INFO)
	log.Println("at info")
}

func TestCapnslogStraight(t *testing.T) {
	plog := NewPackageLogger("github.com/coreos/pkg/capnslog", "main")
	SetFormatter(NewStringFormatter(os.Stdout))
	plog.Error("error")
	plog.Print("print")
	plog.Info("info")
	plog.Debug("debug")
}
