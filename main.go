package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type bucketObjectList struct {
	Contents []bucketObject
}

type bucketObject struct {
	Key  string
	ETag string
}

var (
	showVersion bool
	version     = "dev"
	dryRun      bool
	showHelp    bool
	localDir    string
	bucket      string
	baseURL     string
	urls        = make(map[string]struct{})
	outputPath  string
)

func init() {
	flag.BoolVar(&showVersion, "version", false, "print version number")
	flag.BoolVar(&showHelp, "help", false, "show help")
	flag.BoolVar(&dryRun, "dryrun", false, "run command without making changes")
	flag.StringVar(&localDir, "local-dir", "", "the local directory path of files to sync with an S3 bucket")
	flag.StringVar(&bucket, "bucket", "", "the S3 bucket <S3Uri> e.g. s3://bucketname")
	flag.StringVar(&baseURL, "base-url", "", "modified files will be converted to a URL and saved to urls.txt")
	flag.StringVar(&outputPath, "output-path", "urls.txt", "the path where to save modified URLS when using -base-url")
	flag.Usage = usage
}

func main() {
	flag.Parse()

	if showVersion {
		fmt.Printf("%s %s (runtime: %s)\n", os.Args[0], version, runtime.Version())
		os.Exit(0)
	}

	if showHelp || (localDir == "" && bucket == "") {
		flag.Usage()
		os.Exit(0)
	}

	if _, err := os.Stat(localDir); os.IsNotExist(err) {
		fmt.Println("error: the path supplied to the -local-dir option does not exist")
		os.Exit(2)
	}

	if !strings.Contains(bucket, "s3://") {
		fmt.Println("error: the path supplied to the -bucket option is not a valid <S3Uri> (should start with s3://)")
		os.Exit(2)
	}
	sync()
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: s3-sync [OPTIONS]")
	fmt.Fprintln(os.Stderr, "\nOPTIONS:")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "\nENVIRONMENT:")
	fmt.Fprintln(os.Stderr, "  AWS_ACCESS_KEY_ID        the AWS Access Key ID with permission to write to the S3 bucket")
	fmt.Fprintln(os.Stderr, "  AWS_SECRET_ACCESS_KEY    the AWS Secret Access Key with permission to write to the S3 bucket")
	fmt.Fprintln(os.Stderr, "\nEXAMPLE:")
	fmt.Fprintln(os.Stderr, "  export AWS_ACCESS_KEY_ID=<key_id> \n  export AWS_SECRET_ACCESS_KEY=<key>")
	fmt.Fprintln(os.Stderr, "  s3-sync -source-dir www -bucket s3://bucketname -base-url https://example.com")
	fmt.Fprintln(os.Stderr, "")
}

func sync() {
	os.Remove(outputPath)
	args := []string{"s3", "sync", localDir, bucket, "--delete", "--size-only", "--exclude=*.DS_Store"}
	if dryRun {
		args = append(args, "--dryrun")
	}
	cmd := exec.Command("aws", args...)
	process(cmd)

	bucketObjects := getBucketObjects()
	for _, obj := range bucketObjects.Contents {
		fileHash, err := hashFileMD5(localDir + "/" + obj.Key)
		if err != nil {
			// skip trying to compare the local and remote file
			// if it does not exist localy. This should only happen
			// on a dryrun
			if os.IsNotExist(err) {
				continue
			}
			log.Fatal(err)
		}
		// if MD5 hash of local file does not match
		// copy the local file to the S3 bucket
		remoteHash := trimQuotes(obj.ETag)
		if fileHash != remoteHash {
			filePath := localDir + "/" + obj.Key
			remoteFilePath := localDir + "/" + obj.Key
			// no need to copy if it was already copied with previous sync
			if _, ok := urls[remoteFilePath]; ok {
				continue
			}
			args := []string{"s3", "cp", filePath, remoteFilePath}
			if dryRun {
				args = append(args, "--dryrun")
			}
			cmd = exec.Command("aws", args...)
			process(cmd)
		}
	}
}

func process(cmd *exec.Cmd) {
	r, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	done := make(chan struct{})
	scanner := bufio.NewScanner(r)
	go func() {
		// Read line by line and process it
		for scanner.Scan() {
			line := scanner.Text()
			s3Uri := extractS3Uri(line)
			if s3Uri != "" {
				urls[s3Uri] = struct{}{}
				if baseURL != "" {
					url := strings.Replace(s3Uri, bucket, baseURL, 1)
					url = strings.Replace(url, "index.html", "", 1)
					writeURLtoFile(url)
				}
				fmt.Println(line)
			}
		}
		done <- struct{}{}
	}()
	// Start the command and check for errors
	err := cmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	// Wait for all output to be processed
	<-done
	// Wait for the command to finish
	err = cmd.Wait()
	if err != nil {
		log.Fatal(err)
	}
}

func getBucketObjects() bucketObjectList {
	cmd := exec.Command("aws", "s3api", "list-objects-v2", "--bucket", bucketNameNoProtocol(bucket))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = cmd.Stdout
	err := cmd.Run()
	if err != nil {
		fmt.Println(out.String())
		log.Fatal(err)
	}
	// parse into
	var objList bucketObjectList
	err = json.Unmarshal([]byte(out.String()), &objList)
	if err != nil {
		log.Fatal(err)
	}
	return objList
}

func bucketNameNoProtocol(s3Uri string) string {
	if strings.HasPrefix(s3Uri, "s3://") {
		return strings.TrimPrefix(s3Uri, "s3://")
	}
	return s3Uri
}

func extractS3Uri(line string) string {
	split := strings.Split(line, "s3://")
	if len(split) > 1 {
		return "s3://" + split[1]
	}
	return ""
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func hashFileMD5(filePath string) (string, error) {
	var returnMD5String string

	file, err := os.Open(filePath)
	if err != nil {
		return returnMD5String, err
	}

	defer file.Close()
	hash := md5.New()

	if _, err := io.Copy(hash, file); err != nil {
		return returnMD5String, err
	}

	hashInBytes := hash.Sum(nil)[:16]
	returnMD5String = hex.EncodeToString(hashInBytes)
	return returnMD5String, nil
}

func writeURLtoFile(url string) {
	f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	if _, err := f.WriteString(url + "\n"); err != nil {
		log.Println(err)
	}
}
