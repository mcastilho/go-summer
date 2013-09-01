// package extracts and counts all words from learning set;
// output (map[string]int) is saved in Redis DB
package main

import (
	"bufio"
	"fmt"
	"os"
	"nlptk"
	"redis"
)

const (
	SETDIR = "/home/maxabsent/Documents/learning_set/full_texts/"
)

func WordCount(file_path string) map[string]int {
	// extracts, trims from special signs and counts "bare" words in learning set
	file, err := os.Open(file_path)

	if err != nil {
		fmt.Println("Error reading file", file_path)
		os.Exit(1)
	}

	reader := bufio.NewReader(file)
	word_counter := make(map[string]int)

	for bpar, e := reader.ReadBytes('\n'); e == nil; bpar, e = reader.ReadBytes('\n') {
		paragraph := nlptk.Paragraph{0, string(bpar)}
		sentences := paragraph.GetParts()
		
		for _, sentence := range sentences {
			s := nlptk.Sentence{0, sentence}
			words := s.GetParts()

			if len(words) == 0 {
				continue
			}

			for _, word := range words {
				word_counter[word]++
				word_counter["TOTAL"]++
			}
		}
	}
	return word_counter
}

func Store(dictionary map[string]int) {
	// connect to db
	connection := redis.newConnHdl(redis.DefaultSpec())
	connection.connect()
	// for every element in input dictionary
	// check if word exists in db
	//		1. create new row
	//		2. update the old one
	// close connection
	connection.disconnect()
}

func main() {
	dir, err := os.Open(SETDIR)

	if err != nil {
		fmt.Println("Error reading directory", SETDIR)
		os.Exit(1)
	}

	// select all filenames from open directory
	files_slice, err := dir.Readdirnames(0)

	if err != nil {
		fmt.Println("Error reading filenames from directory", SETDIR)
		os.Exit(1)
	}	

	err := dir.Close()

	if err != nil {
		fmt.Println("Error closing directory", SETDIR)
		os.Exit(1)
	}

	// count words from all files
	// and update database dictionary
	c := make(chan map[string]int)
	
	go func() {
		for _, v := range(files_slice) {
			c <- WordCount(SETDIR + v)
		}
		Store(<-c)
	}()

	fmt.Println("Learning succeed!")
}
