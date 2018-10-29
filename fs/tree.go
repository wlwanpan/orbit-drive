package fs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wlwanpan/orbit-drive/common"
	"github.com/wlwanpan/orbit-drive/db"
)

const (
	// File type constants
	FileCode = iota
	DirCode  = iota
)

var (
	// VTree is the root pointer to the virtual tree of the file
	// structure being synchronized.
	VTree *VNode
)

// VNode represents a file structure where each node can be (i) a dir (ii) a file.
// If is a file, Source links to the ipfs hash saved on the network.
type VNode struct {
	// Id is generated from the absolute path and refers to the key used to save to leveldb.
	Id []byte `json:_id`

	// Path holds the absolute path in the os file system <- Need to compress to relative path.
	Path string `json:path`

	// Type represents if the vnode is a file or dir.
	Type int `json:'type'`

	// Links refers all children vnode in the dir.
	Links []*VNode `json:links`

	// Source refers to the ipfs hash generated by the network.
	Source string `json:source`
}

func (vn *VNode) SetAsDir() {
	vn.Type = DirCode
}

func (vn *VNode) SetAsFile() {
	vn.Type = FileCode
}

// SetSource set the vnode source to the cached source if present.
func (vn *VNode) SetSource(s db.FileStore) {
	i := common.ToStr(vn.Id)
	if src, ok := s[i]; ok {
		vn.Source = src
		delete(s, i)
	}
}

// GenChildId returns a hash from the current vnode id and the given path.
func (vn *VNode) GenChildId(p string) []byte {
	i := append(vn.Id, p...)
	return common.HashStr(common.ToStr(i))
}

// InitVTree initialize a new virtual tree (VTree) given an absolute path.
func InitVTree(path string, s db.FileStore) error {
	VTree = &VNode{
		Path:   path, // To optimize here -> start with "/" not abs path
		Id:     common.ToByte(common.ROOT_KEY),
		Type:   DirCode,
		Links:  []*VNode{},
		Source: "",
	}

	err := VTree.PopulateNodes(s)
	if err != nil {
		return err
	}
	return nil
}

// NewFile traverse VTree to locate path parent dir and
// add a new vnode.
func NewFile(path string) error {
	dir := filepath.Dir(path)
	vn, err := VTree.FindChildAt(dir)
	if err != nil {
		return err
	}
	n := vn.NewVNode(path)
	isDir, err := common.IsDir(path)
	if err != nil {
		return err
	}
	if isDir {
		n.SetAsDir()
		// Read file content and upload
		n.PopulateNodes(db.FileStore{})
	} else {
		n.SetAsFile()
		n.Save()
	}
	return nil
}

// NewVNode initialize and returns a new VNode under current vnode.
func (vn *VNode) NewVNode(path string) *VNode {
	i := append(vn.Id, path...)
	n := &VNode{
		Id:     common.HashStr(common.ToStr(i)),
		Path:   path,
		Links:  []*VNode{},
		Source: "",
	}
	vn.Links = append(vn.Links, n)
	return n
}

// PopulateNodes read a path and populate the its links given
// the path is a directory else creates a file node.
func (vn *VNode) PopulateNodes(s db.FileStore) error {
	files, err := ioutil.ReadDir(vn.Path)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, f := range files {
		abspath := vn.Path + "/" + f.Name()
		n := vn.NewVNode(abspath)
		n.SetSource(s)

		if f.IsDir() {
			n.SetAsDir()
			n.PopulateNodes(s)
		} else {
			n.SetAsFile()
			wg.Add(1)
			go func() {
				err := n.Save()
				if err != nil {
					// To write to a log file.
					fmt.Println(err)
				}
				wg.Done()
			}()
		}
	}

	wg.Wait()
	return nil
}

// Save
func (vn *VNode) Save() error {
	// If ipfs hash not present, then upload to network
	if vn.Source == "" {
		s, err := UploadFile(vn.Path)
		if err != nil {
			fmt.Println(err)
			return err
		}
		vn.Source = s
	}
	return db.Db.Put(vn.Id, common.ToByte(vn.Source), nil)
}

// FindChildAt perform a full traversal to look a vnode from a given path.
func (vn *VNode) FindChildAt(path string) (*VNode, error) {
	if vn.Path == path {
		return vn, nil
	}
	return vn.Traverse(path)
}

func (vn *VNode) Traverse(path string) (*VNode, error) {
	rel, err := filepath.Rel(vn.Path, path)
	if err != nil {
		return vn, err
	}
	dir := filepath.Dir(rel)
	steps := strings.Split(dir, "/")
	currNode := vn

	for _, step := range steps {
		pathToFind := currNode.Path + step
		_id := currNode.GenChildId(pathToFind)
		link, err := currNode.FindChild(_id)
		if err != nil {
			return vn, err
		}
		currNode = link
	}

	return currNode, nil
}

// FindChild look for a given id from its Links (1 level).
func (vn *VNode) FindChild(i []byte) (*VNode, error) {
	if vn.Type == FileCode {
		return vn, ErrNotADir
	}

	for _, n := range vn.Links {
		if bytes.Equal(n.Id, i) {
			return n, nil
		}
	}
	return vn, ErrVNodeNotFound
}

// RemoveNode traverse a VTree and remove the VNode at the given path.
func (vn *VNode) UnlinkChild(path string) error {
	return nil
}
