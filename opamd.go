package main

import (
    "net/http"
    "fmt"
    "strings"
    "path"
    "path/filepath"
    "os"
    "os/exec"
    "errors"
    "io"
    "time"
    "flag"
    "gconf/conf"
)

var repo repository
var eseguible map[string]string

func main() {
    // get configuration
    port := "8083"
    remoterepo := "http://opam.kino3d.org/hg"
    
    var confPath = flag.String("conf", "./opamd.conf", "path to configuration file")
    
    flag.Parse()
    
    confFile, err := conf.ReadConfigFile(*confPath)
    if err != nil {
        fmt.Print(err)
        return
    }
    
    eseguible = make(map[string]string, 0)
    
    confSections := confFile.GetSections()
    
    for _, s := range(confSections) {

        extensions, err := confFile.GetString(s, "extensions")
        if err != nil {
            continue
        }
        
        path, err := confFile.GetString(s, "path")
        if err != nil {
            path = s
        }
        extensions = strings.Replace(extensions, " ", "", -1)
        
        for _, ext := range(strings.Split(extensions, ",")){
            eseguible[ext] = path
        }
    }

    localrepo, err := confFile.GetString("defaults", "localrepo")
    if err != nil {
        fmt.Printf("can't get value for localrepo, check config file, default section\n")
    }
    repo.path = localrepo
    
    repo.remotePath = remoterepo
    repo.inProgress = false

    // listen for requests on localhost
    fmt.Printf("configuration is copleted, now listening for requests.\n")
    http.HandleFunc("/", ServeLocalData)
    fmt.Print(http.ListenAndServe("localhost:" + port, nil))
}

func ServeLocalData(out http.ResponseWriter, in *http.Request) {

    command := in.URL.Path[1:]

    var filePath string
    var projectName string

    if command == "open" {
        err := in.ParseForm()
        if err != nil {
            fmt.Print(err)
            return
        }

        for k, _ := range in.Form {
            if k == "" {
                fmt.Printf("no path was found in request data\n")
                break
            }
            index := strings.Index(k, "/")
            if index < 0 {
                fmt.Printf("requested path is to short\n")
                break
            }
            projectName = k[:index]
            filePath = k[index+1:]
            break
        }

        if filePath == ""  || projectName == "" {
            fmt.Printf("no enought info in request path\n")
            return
        }

        // update repo
        err = repo.Update(projectName)
        if err != nil {
            fmt.Print(err)
            fmt.Printf("error when updating repository")
            return
        }

        // create working copy of file
        completeWorkingFilePath, err := updateWorkingCopy(repo.path, projectName, filePath)
        if err != nil {
            if os.IsNotExist(err) {
            } else {
                fmt.Print(err)
                return
            }
        }

        fileExt := filepath.Ext(completeWorkingFilePath)
        fileExt = fileExt[1:]
        
        cmd, ok := eseguible[fileExt]
        if !ok {
            fmt.Printf("no programm was configured to open a %s file\n", fileExt)
            fmt.Printf("please check you config file\n")
        }
        
        err = runCmd(cmd, completeWorkingFilePath)

    }
}

func updateWorkingCopy(repoPath, projectName, filePath string) (string, error) {
    // get name
    fileName := path.Base(filePath)
    workingFileName := "@" + fileName
    workingFilePath := strings.Replace(filePath, fileName, workingFileName, 1)

    completeFilePath := path.Join(repoPath, projectName, filePath)
    completeWorkingFilePath := path.Join(repoPath, projectName, workingFilePath)

    src, err := os.Open(completeFilePath)
    if err != nil {
        return completeWorkingFilePath, err
    }
    defer src.Close()

    srcStat, _ := src.Stat()
    srcModTime := srcStat.ModTime()

    dstModTime := srcModTime

    dst, err := os.Open(completeWorkingFilePath)
    if err != nil {
        dstModTime = time.Unix(1, 1)
    } else {
        dstStat, _ := dst.Stat()
        dstModTime = dstStat.ModTime()
        dst.Close()
    }

    if dstModTime.Before(srcModTime) {
        fmt.Printf("need to update file\n")

        dst, err := os.Create(completeWorkingFilePath)
        if err != nil {
            return "", err
        }
        defer dst.Close()

        _, err = io.Copy(dst, src)
        if err != nil {
            return "", err
        }
    } else {
        fmt.Printf("working copy is newver that file in repository\n")
    }

    return completeWorkingFilePath, nil
}

func runCmd(command string, args...string) error {
    cmd := exec.Command(command, args...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    err := cmd.Run()
    if err != nil {
        return err
    }
    return nil
}

type repository struct {
    path string
    remotePath string
    inProgress bool
}

func (r *repository) Update(projectName string) error{

    if r.inProgress {
        fmt.Printf("another instance has blocked this repo. Retry after a while.")
        return errors.New("update in progress by another instance")
    }

    r.inProgress = true
    defer func() { r.inProgress = false; }()

    // check if repo was aleardy cloned
    cloned := true

    fd, err := os.Stat(path.Join(r.path, projectName))
    if err != nil {
        if os.IsNotExist(err) {
            cloned = false
        }
    } else {
		if !fd.IsDir() {
			fmt.Printf("found file, need directory\n")
			return errors.New("found file, need directory\n")
			}
	}

    if !cloned {
        fmt.Printf("need to clone project repo\n")
        err = runCmd("hg", "clone", r.remotePath + "/" + projectName, path.Join(r.path, projectName))
        if err != nil {
            return err
        }
    } else {
        fmt.Printf("can be updated\n")
        err = runCmd("hg", "pull", "-u", "-R", path.Join(r.path, projectName))
        if err != nil {
            return err
        }
    }
    return nil
}
