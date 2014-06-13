package zk

import (
	"errors"
	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
	"path"
	"strings"
	"time"
)

var (
	ErrNoChild      = errors.New("zk: children is nil")
	ErrNodeNotExist = errors.New("zk: node not exist")
)

func Connect(addr []string, timeout time.Duration) (*zk.Conn, error) {
	conn, session, err := zk.Connect(addr, timeout)
	if err != nil {
		glog.Errorf("zk.Connect(\"%v\", %d) error(%v)", addr, timeout, err)
		return nil, err
	}
	go func() {
		for {
			event := <-session
			glog.Infof("zookeeper get a event: %s", event.State.String())
		}
	}()
	return conn, nil
}

func Create(conn *zk.Conn, fpath string) error {
	tpath := ""
	for _, str := range strings.Split(fpath, "/")[1:] {
		tpath = path.Join(tpath, "/", str)
		glog.V(1).Infof("create zookeeper path: \"%s\"", tpath)
		_, err := conn.Create(tpath, []byte(""), 0, zk.WorldACL(zk.PermAll))
		if err != nil {
			if err == zk.ErrNodeExists {
				glog.Warningf("zk.create(\"%s\") exists", tpath)
			} else {
				glog.Errorf("zk.create(\"%s\") error(%v)", tpath, err)
				return err
			}
		}
	}
	return nil
}

// 创建一个临时节点，并监听自己
func CreateTempW(conn *zk.Conn, fpath, data string, watchFunc func(bool, zk.Event)) error {
	tpath, err := conn.Create(path.Join(fpath)+"/", []byte(data), zk.FlagEphemeral|zk.FlagSequence, zk.WorldACL(zk.PermAll))
	if err != nil {
		glog.Errorf("conn.Create(\"%s\", \"%s\", zk.FlagEphemeral|zk.FlagSequence) error(%v)", fpath, data, err)
		return err
	}
	glog.V(1).Infof("create a zookeeper node:%s", tpath)
	// watch self
	if watchFunc != nil {
		go func() {
			for {
				glog.Infof("zk path: \"%s\" set a watch", tpath)
				exist, _, watch, err := conn.ExistsW(tpath)
				if err != nil {
					glog.Errorf("zk.ExistsW(%s) error(%v)", tpath, err)
					return
				}
				event := <-watch
				watchFunc(exist, event)
				glog.Infof("zk path: \"%s\" receive a event %v", tpath, event)
			}
		}()
	}
	return nil
}

// 获取节点信息，并监听实践，如果一直监听，必须自己实现
func GetNodesW(conn *zk.Conn, path string) ([]string, <-chan zk.Event, error) {
	nodes, stat, watch, err := conn.ChildrenW(path)
	if err != nil {
		if err == zk.ErrNoNode {
			return nil, nil, ErrNodeNotExist
		}
		glog.Errorf("zk.ChildrenW(\"%s\") error(%v)", path, err)
		return nil, nil, err
	}
	if stat == nil {
		return nil, nil, ErrNodeNotExist
	}
	if len(nodes) == 0 {
		return nil, nil, ErrNoChild
	}
	return nodes, watch, nil
}

// 得到节点的数据
func GetNodeData(conn *zk.Conn, path string) (string, error) {
	data, _, err := conn.Get(path)
	return string(data), err
}

func GetNodes(conn *zk.Conn, path string) ([]string, error) {
	nodes, stat, err := conn.Children(path)
	if err != nil {
		if err == zk.ErrNoNode {
			return nil, ErrNodeNotExist
		}
		glog.Errorf("zk.Children(\"%s\") error(%v)", path, err)
		return nil, err
	}
	if stat == nil {
		return nil, ErrNodeNotExist
	}
	if len(nodes) == 0 {
		return nil, ErrNoChild
	}
	return nodes, nil
}
