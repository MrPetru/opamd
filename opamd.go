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
)

var repo repository
var imageEditor = "gimp"
var blendEditor = "blender"
var videoPlayer = "ffplay"

func main() {
    // get configuration
    port := ":8083"
    
    //repo = new(repository)
    repo.path = "/tmp"
    repo.remotePath = "http://opam.kino3d.org/hg"
    repo.inProgress = false
    
    // listen for requests on localhost
    http.HandleFunc("/", ServeLocalData)
    fmt.Print(http.ListenAndServe("localhost"+port, nil))
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
            //fmt.Printf("project = %s \nfile = %s\n", projectName, filePath)
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
            fmt.Print(err)
            return
        }
        
        fmt.Print(completeWorkingFilePath)
        
        fileExt := filepath.Ext(completeWorkingFilePath)
        fileExt = fileExt[1:]
        fmt.Print(fileExt)
        
        switch fileExt = strings.ToLower(fileExt); fileExt {
            case "jpg", "png", "xcf", "tif", "tiff", "jpeg":
                err = runCmd(imageEditor, completeWorkingFilePath)
            case "blend":
                err = runCmd(blendEditor, completeWorkingFilePath)
            case "mov", "mp4", "avi", "mv2", "mts", "mxf":
                err = runCmd(videoPlayer, completeWorkingFilePath)
        }
    }  
}

func updateWorkingCopy(repoPath, projectName, filePath string) (string, error) {
    // get name
    fileName := path.Base(filePath)
    workingFileName := "@" + fileName
    workingFilePath := strings.Replace(filePath, fileName, workingFileName, 1)
    
    completeFilePath := path.Join(repoPath, projectName, filePath)
    completeWorkingFilePath := path.Join(repoPath, projectName, workingFilePath)
    
    //fmt.Printf("%s %s", workingFileName, workingFilePath)
    
    src, err := os.Open(completeFilePath)
    if err != nil {
        return "", err
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
        } else {
        if !fd.IsDir() {
            fmt.Printf("found file, need directory\n")
            return errors.New("found file, need directory\n")
            }
        }
    }

    if !cloned {
        fmt.Printf("need to clone project repo\n")
        err = runCmd("hg", "clone", r.remotePath + "/" + projectName, r.path + "/" + projectName)
        if err != nil {
            return err
        }
    } else {
        fmt.Printf("can be updated\n")
        err = runCmd("hg", "pull", "-u", "-R", r.path + "/" + projectName)
        if err != nil {
            return err
        }
    }
    return nil
}
