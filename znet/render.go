/*
 * @Author: seekwe
 * @Date:   2019-05-23 19:16:32
 * @Last Modified by:   seekwe
 * @Last Modified time: 2019-05-29 15:40:30
 */

package znet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sohaha/zlsgo/zfile"
	"github.com/sohaha/zlsgo/zstring"
	"github.com/sohaha/zlsgo/zutil"
)

type (
	render interface {
		Content(c *Context) (content []byte)
	}
	renderByte struct {
		Data        []byte
		Type        string
		ContentDate []byte
	}
	renderString struct {
		Format      string
		Data        []interface{}
		ContentDate []byte
	}
	renderJSON struct {
		Data        interface{}
		ContentDate []byte
	}
	renderFile struct {
		Data        string
		FileExist   bool
		ContentDate []byte
	}
	renderHTML struct {
		Template    *template.Template
		Name        string
		Data        interface{}
		ContentDate []byte
		FuncMap     template.FuncMap
	}
	Api struct {
		Code int         `json:"code" example:"200"`
		Msg  string      `json:"msg"`
		Data interface{} `json:"data"`
	}
	Data     map[string]interface{}
	PrevData struct {
		Code    int
		Type    string
		Content []byte
	}
)

var (
	ContentTypePlain = "text/plain; charset=utf-8"
	ContentTypeHTML  = "text/html; charset=utf-8"
	ContentTypeJSON  = "application/json; charset=utf-8"
)

func (c *Context) renderProcessing(code int, r render) {
	c.Lock()
	c.Code = code
	c.render = r
	c.StopHandle = true
	c.Unlock()
}

func (r *renderByte) Content(c *Context) []byte {
	c.SetContentType(zutil.IfVal(r.Type != "", r.Type, ContentTypePlain).(string))
	return r.Data
}

func (r *renderString) Content(c *Context) []byte {
	if r.ContentDate != nil {
		return r.ContentDate
	}
	c.SetContentType(ContentTypePlain)
	if len(r.Data) > 0 {
		r.ContentDate = zstring.String2Bytes(fmt.Sprintf(r.Format, r.Data...))
	} else {
		r.ContentDate = zstring.String2Bytes(r.Format)
	}
	return r.ContentDate
}

func (r *renderJSON) Content(c *Context) []byte {
	if r.ContentDate != nil {
		return r.ContentDate
	}
	c.SetContentType(ContentTypeJSON)
	r.ContentDate, _ = json.Marshal(r.Data)
	return r.ContentDate
}

func (r *renderFile) Content(c *Context) []byte {
	if !r.FileExist {
		return []byte{}
	}

	if r.ContentDate != nil {
		return r.ContentDate
	}

	fType := mime.TypeByExtension(filepath.Ext(r.Data))
	c.SetContentType(fType)
	r.ContentDate, _ = ioutil.ReadFile(r.Data)
	return r.ContentDate
}

func (r *renderHTML) Content(c *Context) []byte {
	if r.ContentDate != nil {
		return r.ContentDate
	}
	c.SetContentType(ContentTypeHTML)
	if r.Name != "" {
		var t *template.Template
		var err error
		t, err = templateParse(r.Name, r.FuncMap)
		if err != nil {
			panic(err)
		}
		var buf bytes.Buffer
		err = t.Execute(&buf, r.Data)
		if err != nil {
			panic(err)
		}
		r.ContentDate = buf.Bytes()
	} else {
		r.ContentDate = zstring.String2Bytes(fmt.Sprint(r.Data))
	}
	return r.ContentDate
}

func (c *Context) Byte(code int, value []byte) {
	c.renderProcessing(code, &renderByte{Data: value})
}

func (c *Context) String(code int, format string, values ...interface{}) {
	c.renderProcessing(code, &renderString{Format: format, Data: values})
}

func (c *Context) SetContent(data *PrevData) {
	c.renderProcessing(data.Code, &renderByte{Data: data.Content, Type: data.Type})
}

func (c *Context) File(path string) {
	fileExist := zfile.FileExist(path)
	code := zutil.IfVal(fileExist, 200, 404).(int)
	if fileExist {
		if f, err := os.Stat(path); err == nil {
			c.SetHeader("Last-Modified", f.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))
		}
	}
	c.renderProcessing(code, &renderFile{Data: path, FileExist: fileExist})
}

func (c *Context) JSON(code int, values interface{}) {
	c.renderProcessing(code, &renderJSON{Data: values})
}

// ResJSON ResJSON
func (c *Context) ResJSON(code int, msg string, data interface{}) {
	httpState := code
	if code < 300 && code >= 200 {
		httpState = http.StatusOK
	}
	c.renderProcessing(httpState, &renderJSON{Data: Api{Code: code, Data: data, Msg: msg}})
}

// HTML export html
func (c *Context) HTML(code int, html string) {
	c.renderProcessing(code, &renderHTML{
		Name: "",
		Data: html,
	})
}

// Template export template
func (c *Context) Template(code int, name string, data interface{}, funcMap ...map[string]interface{}) {
	var fn template.FuncMap
	if len(funcMap) > 0 {
		fn = funcMap[0]
	}
	c.renderProcessing(code, &renderHTML{
		Name:    name,
		Data:    data,
		FuncMap: fn,
	})
}

// Abort Abort
func (c *Context) Abort(code ...int) {
	c.Lock()
	c.StopHandle = true
	c.Unlock()
	if len(code) > 0 {
		c.SetStatus(code[0])
	}
}

// Redirect Redirect
func (c *Context) Redirect(link string, statusCode ...int) {
	c.Writer.Header().Set("Location", c.CompletionLink(link))
	code := http.StatusFound
	if len(statusCode) > 0 {
		code = statusCode[0]
	}
	c.SetStatus(code)
}

func (c *Context) SetStatus(code int) *Context {
	c.Lock()
	c.Code = code
	c.Unlock()
	return c
}

func (c *Context) SetContentType(contentType string) *Context {
	c.SetHeader("Content-Type", contentType)
	return c
}

func (c *Context) PrevContent() *PrevData {
	c.RLock()
	r := c.render
	code := c.Code
	ctype, hasType := c.heades["Content-Type"]
	c.RUnlock()
	if !hasType {
		ctype = ContentTypePlain
	}
	var content []byte
	if r != nil {
		content = r.Content(c)
	}
	return &PrevData{
		Code:    code,
		Type:    ctype,
		Content: content,
	}
}
