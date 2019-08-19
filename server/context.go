package server

import "net/http"
import "os"
import "path"
import "path/filepath"
import "strings"
import "fmt"
import "net/url"
import "github.com/aws/aws-sdk-go/aws/session"
import "github.com/aws/aws-sdk-go/service/s3"
import "github.com/aws/aws-sdk-go/aws"
import "github.com/aws/aws-sdk-go/aws/credentials"
import "github.com/aws/aws-sdk-go/service/s3/s3manager"


// context contains information about request path
type context struct {
    cleanPath       string  // clean path relative to root
    url             string  // cleanPath + query string (used to do 307 redirect if r.url is not clean)
    absFilePath     string  // absolute path pointing to a file or dir of the disk
    exist           bool
    isDir           bool
    indexPath       string  // if path is a directory, detect index path
                            // indexPath is a path contains index name and relative to root
                            // indexPath == path.Join(cleanPath, indexName)
}

func DownloadFromS3Bucket(item string) (error) {
    bucket := "caddyimages"

    file, err := os.Create(item)
    if err != nil {
        fmt.Println(err)
    }

    defer file.Close()

    sess, _ := session.NewSession(&aws.Config{
        Region:      aws.String("us-east-2"),
        Credentials: credentials.NewSharedCredentials("~/.aws/credetials", "default"),
    })

    downloader := s3manager.NewDownloader(sess)

    _, err = downloader.Download(file,
        &s3.GetObjectInput{
            Bucket: aws.String(bucket),
            Key:    aws.String(item),
        })
    if err != nil {
        fmt.Println(err)
    }

    return err
}

// String() is used for log output
func (c *context) String() string {
    return fmt.Sprintf("cleanPath: %s, url: %s, absFilePath: %s, exist: %t, isDir: %t, indexPath: %s",
        c.cleanPath, c.url, c.absFilePath, c.exist, c.isDir, c.indexPath)
}


// Make a new context
func newContext(config Config, r *http.Request) (c *context, err error) {
    c = new(context)

    requestPath := r.URL.Path

    if !strings.HasPrefix(requestPath, "/") {
        requestPath = "/" + requestPath
    }
    c.cleanPath = path.Clean(requestPath)
    s3Path := c.cleanPath[1:]

    c.absFilePath, err = filepath.Abs(filepath.Join(config.Root, c.cleanPath))
    if err != nil {
        return
    }
    if len(s3Path) > 4{
      err = DownloadFromS3Bucket(s3Path)
      if err != nil {
        c.exist = false
      }
    }

    info, e := os.Stat(c.absFilePath)

    if e != nil {
        if os.IsNotExist(e) {
            c.exist = false
        } else {
            err = e
            return
        }
    } else {
        c.exist = true
        c.isDir = info.IsDir()
    }

    // if -serve-all is false and the path is a hidden path, then return 404 error.
    // a hidden path is a path start with dot.
    if !config.ServeAll && strings.Contains(c.cleanPath, "/.") {
        c.exist = false
        c.isDir = false
    }

    if c.isDir {
        for _, name := range config.IndexName {
            index := filepath.Join(c.absFilePath, name)
            indexInfo, e := os.Stat(index)
            if e != nil {
                if os.IsNotExist(e) {
                    continue
                } else {
                    err = e
                    return
                }
            }

            if indexInfo.IsDir() {
                continue
            } else {
                c.isDir = false
                c.indexPath = path.Join(c.cleanPath, name)
                c.absFilePath, err = filepath.Abs(filepath.Join(config.Root, c.indexPath))
                if err != nil {
                    return
                }
                // add trailing slash if the request path is a directory and the directory contains a index file
                if !strings.HasSuffix(c.cleanPath, "/") {
                    c.cleanPath += "/"
                }
                break
            }
        }

        if !config.ListDir && c.indexPath == "" {
            c.exist = false
        }
    }

    c.url = c.cleanPath
    if c.isDir && !strings.HasSuffix(c.url, "/") {
        c.url += "/"
    }

    // use net/url package to escape url
    newurl := url.URL{Path: c.url, RawQuery:r.URL.RawQuery}
    c.url = newurl.String()

    return
}


// Get parent from a url. Parameter url is start with "/".
func (this *context) parent() string {
    u := this.url

    if u == "/" {
        return "/"
    }

    // remove query string (? and the string after it)
    if i := strings.LastIndex(u, "?"); i > 0 {
        u = u[:i]
    }

    // remove last "/"
    if strings.HasSuffix(u, "/") {
        u = u[:len(u) - 1]
    }

    i := strings.LastIndex(u, "/")
    if i <= 0 {
        return "/"
    }

    return u[:i + 1]
}
