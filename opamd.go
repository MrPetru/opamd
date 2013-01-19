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
    "encoding/json"
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
    fmt.Printf("configuration complete, now listening for requests.\n")
    http.HandleFunc("/open", ServeLocalData)
    http.HandleFunc("/createdirectory", CreateAssetDirectory)
    fmt.Print(http.ListenAndServe("localhost:" + port, nil))
}

type resultMessage struct {
    Status string `json:"status"`
    Msg string `json:"msg"`
    Updates []string `json:"updates"`
}

func CreateAssetDirectory(out http.ResponseWriter, in *http.Request) {

    var filePath string
    var projectName string

    err := in.ParseForm()
    if err != nil {
        fmt.Print(err)
        SendResult(out,  in, "error", "unknown command")
        return
    }

    if p, ok := in.Form["path"]; ok && len(p) == 1 {
        filePath = p[0]
    }
    
    index := strings.Index(filePath, "/")
    if index < 0 {
        fmt.Printf("requested path is to short\n")
        SendResult(out,  in, "error", "requested path is to short")
        return
    }
    
    projectName = filePath[:index]
    filePath = filePath[index+1:]

    if filePath == ""  || projectName == "" {
        fmt.Printf("no enough info in request path\n")
        SendResult(out,  in, "error", "no enough info in request path")
        return
    }
    
    directory := filepath.Dir(filePath)
    
    completeDirectoryPath := filepath.Join(repo.path, projectName, directory)
    
    _, err = os.Stat(path.Join(repo.path, projectName))
    if err != nil {
        if os.IsNotExist(err) {
            return
        }
    }
    
    // create all directories
    err = os.MkdirAll(completeDirectoryPath, 0777)
    if err != nil {
        fmt.Printf("on creating folders: %v\n", err)
    }
    
    SendResult(out,  in, "ok", "")
    return
}

func ServeLocalData(out http.ResponseWriter, in *http.Request) {

    var filePath string
    var projectName string

    err := in.ParseForm()
    if err != nil {
        fmt.Print(err)
        SendResult(out,  in, "error", "unknown command")
        return
    }

    if p, ok := in.Form["path"]; ok && len(p) == 1 {
        filePath = p[0]
    }
    
    index := strings.Index(filePath, "/")
    if index < 0 {
        fmt.Printf("requested path is to short\n")
        SendResult(out,  in, "error", "requested path is to short")
        return
    }
    
    projectName = filePath[:index]
    filePath = filePath[index+1:]

    if filePath == ""  || projectName == "" {
        fmt.Printf("no enough info in request path\n")
        SendResult(out,  in, "error", "no enough info in request path")
        return
    }

    // update repo
    err = repo.Update(projectName)
    if err != nil {
        fmt.Print(err)
        fmt.Printf("error when updating repository")
        SendResult(out,  in, "error", err.Error())
        return
    }

    // create working copy of file
    completeWorkingFilePath, err := updateWorkingCopy(repo.path, projectName, filePath)
    if err != nil {
        if os.IsNotExist(err) {
            SendResult(out,  in, "error", err.Error())
            return
        } else {
            SendResult(out,  in, "error", err.Error())
            return
        }
    }

    fileExt := filepath.Ext(completeWorkingFilePath)
    fileExt = fileExt[1:]
    
    cmd, ok := eseguible[fileExt]
    if !ok {
        fmt.Printf("no programm was configured to open a %s file\n", fileExt)
        fmt.Printf("please check you config file\n")
        SendResult(out,  in, "error", "unknown file type, check config file")
        return
    }
    
    SendResult(out,  in, "ok", "editing file " + filePath)
    
    err = runCmd(cmd, completeWorkingFilePath)
    if err != nil {
        fmt.Printf("error is = %v", err)
    }
}

func SendResult(out http.ResponseWriter, in *http.Request, status, msg string) {

    message := new(resultMessage)
    message.Updates = make([]string, 0)
    
    message.Status = status
    message.Msg = msg
    
    jsonMessage, _ := json.Marshal(message)
    
    conn, _, err := out.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(out, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Don't forget to close the connection:
	defer conn.Close()
    const TimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"
    
    conn.Write([]byte("HTTP/1.0 200 OK\n"))
    conn.Write([]byte("Access-Control-Allow-Origin: " + in.Header["Origin"][0] + "\n"))
    conn.Write([]byte("Content-Type: application/json; charset=utf-8\n"))
    conn.Write([]byte("Date: " + time.Now().UTC().Format(TimeFormat) + "\n"))
    conn.Write([]byte("Transfer-Encoding: chunked\n\n"))

    conn.Write([]byte(jsonMessage))
    
//    out.Header().Set("Content-Type","application/json; charset=utf-8")
//    out.Header().Set("Access-Control-Allow-Origin", "http://localhost:8081")
//    fmt.Println(out.Header())
//    fmt.Fprint(out, string(jsonMessage))
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
        fmt.Printf("%v\n", err)
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
        fmt.Printf("updating working file...\n")

        dst, err := os.Create(completeWorkingFilePath)
        if err != nil {
            fmt.Printf("%v\n", err)
            return "", err
        }
        defer dst.Close()

        _, err = io.Copy(dst, src)
        if err != nil {
            fmt.Printf("%v\n", err)
            return "", err
        }
    } else {
        fmt.Printf("working file doesn't need an update\n")
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
        fmt.Printf("repo update in progress by another instance\n")
        return errors.New("repo update in progress by another instance")
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
			return errors.New("found a file, need directory\n")
			}
	}

    if !cloned {
        fmt.Printf("cloning proj to local repo\n")
        err = runCmd("hg", "clone", r.remotePath + "/" + projectName, path.Join(r.path, projectName))
        if err != nil {
            fmt.Printf("error when coloning project to local repo (%s)\n", err.Error())
            return errors.New("error when coloning project to local repo (" + err.Error() + ")")
        }
    } else {
        fmt.Printf("updating...\n")
        err = runCmd("hg", "pull", "-u", "-R", path.Join(r.path, projectName))
        if err != nil {
            fmt.Printf("error on update project (%s)\n", err.Error())
            return errors.New("error on update project (" + err.Error() + ")")
        }
    }
    return nil
}
