package main

import (
	"log"

	jellybean "github.com/AngelVI13/jelly-bean"
)

var args struct {
	UseRegex bool   `arg:"-r,--regex" default:"false" help:"Flag that enables regex search"`
	FileType string `arg:"-t,--type" default:".py" help:"Filetypes to search (i.e. '.py')"`

	Distance int `arg:"-d,--dist" default:"6" help:"Levels of recursive search"`

	LogFile string `arg:"-l,--log" default:"search.log" help:"Log filename"`
	OutFile string `arg:"-o,--out" default:"search_tc.xml" help:"Output xml filename"`
	WiFile  string `arg:"-w,--wi" default:"" help:"Exported XML file from polarion containing all TCA work item info."`

	Pattern string `arg:"positional,required" help:"Pattern to search for"`
	Dir     string `arg:"positional,required" help:"Directory to search in"`
}

func main() {
	jellybean.MustParse(&args)

	log.Println("UseRegex", args.UseRegex)
	log.Println("FileType", args.FileType)

	log.Println("Distance", args.Distance)

	log.Println("LogFile", args.LogFile)
	log.Println("OutFile", args.OutFile)
	log.Println("WiFile", args.WiFile)

	log.Println("Pattern", args.Pattern)
	log.Println("Dir", args.Dir)

}
