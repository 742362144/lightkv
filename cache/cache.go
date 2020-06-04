package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

func isExist(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

func createDir(path string) error {
	if !isExist(path) {
		err := os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return err
		}
		return err
	}
	return nil
}


type OpType int32

const (
	Add = 0
	Del = 1
)

type persistentValueOp struct {
	item   Value
	opType OpType
}

type persistentMapOp struct {
	item   MapValue
	opType OpType
}

type Cache struct{
	caches map[string]Value
	mapCaches map[string]MapValue

	checkExpireInterval int
	persistentChan chan persistentValueOp
	persistentMapChan chan persistentMapOp

	opFunction func(OpType, DataString)
	mutex sync.RWMutex
	mapMutex sync.RWMutex
}

const ExpireForever = 0
var DefaultDBPath = "db"
var ValueDBPath = path.Join(DefaultDBPath, "Value")
var MapDBPath = path.Join(DefaultDBPath, "map")

func NewCache(checkExpireInterval int) *Cache {
	 c := Cache{
	 	caches: make(map[string]Value),
	 	mapCaches:make(map[string]MapValue),
	 	checkExpireInterval:checkExpireInterval,
	 	persistentChan:make(chan persistentValueOp),
	 	persistentMapChan:make(chan persistentMapOp),
	 	opFunction:nil,
	 }
	 c.init()
	 return &c
}

func (s*Cache) init() {
	os.Mkdir(DefaultDBPath, os.ModePerm)
	os.Mkdir(ValueDBPath, os.ModePerm)
	os.Mkdir(MapDBPath, os.ModePerm)

	s.loadDB()

	go s.persistent()
	go s.checkExpire()
}

func (s *Cache) loadDB()  {

	 //普通类型
	 filepath.Walk(ValueDBPath, func(path string, f os.FileInfo, err error) error {
		if f == nil {
			return err
		}
		if f.IsDir() {
			return nil
		}

		 if data, err := ioutil.ReadFile(path); err != nil {
			 log.Println(err)
		 }else {
		 	 v := decode(data)
			 s.caches[v.Key] = v
		 }
		return nil
	})

	//map类型
	filepath.Walk(MapDBPath, func(path string, f os.FileInfo, err error) error {
		if f == nil {
			return err
		}
		if f.IsDir() {
			return nil
		}

		if data, err := ioutil.ReadFile(path); err != nil {
			log.Println(err)
		}else {
			v := decodeHM(data)
			s.mapCaches[v.Key] = v
		}
		return nil
	})

	log.Printf("load db finish, %d Key-cacheValue ", len(s.caches)+len(s.mapCaches))
}

func (s*Cache) SetOnOP(opFunc func(OpType, DataString)) {
	s.opFunction = opFunc
}

func (s*Cache) Put(key string, v string, expire int64 ) error{
	s.mutex.Lock()
	old, has := s.caches[key]
	var needUpdate bool = false
	var val Value
	if expire == ExpireForever {
		val = Value{Key: key, Data: v, Expire:ExpireForever}

		if has {
			if old.Data == val.Data && old.Expire == val.Expire{
				needUpdate = false
			}else{
				needUpdate = true
			}
		}else{
			needUpdate = true
		}
		s.caches[key] = val

		if s.opFunction != nil{
			s.opFunction(Add, &val)
		}
	}else{
		needUpdate = true
		e := time.Now().UnixNano() + expire*int64(time.Second)
		val = Value{Key: key, Data: v, Expire:e}
		s.caches[key] = val
		if s.opFunction != nil{
			s.opFunction(Add, &val)
		}
	}
	s.mutex.Unlock()

	log.Printf("put Key:%s, Value:%v, expire:%d", key, v, expire)

	if needUpdate {
		op := persistentValueOp{item: val, opType:Add}
		s.persistentChan <- op
	}

	return nil
}

func (s *Cache) Get(key string) (string, error) {
	s.mutex.RLock()
	v, ok := s.caches[key]
	s.mutex.RUnlock()
	if ok{
		t := time.Now().UnixNano()
		if v.Expire != ExpireForever && v.Expire <= t{
			str := fmt.Sprintf("get Key:%s, not found ", key)
			return "", errors.New(str)
		}else{
			str := fmt.Sprintf("get Key:%s, Value:%s", key, v.Data)
			log.Println(str)
			return v.Data, nil
		}
	}else{
		str := fmt.Sprintf("get Key:%s, not found", key)
		return "", errors.New(str)
	}
}

func (s *Cache) Delete (key string) error{
	s.mutex.Lock()
	s.del(key)
	s.mutex.Unlock()
	return nil
}

func (s *Cache) HMPut(hmKey string, keys [] string,  fields [] string, expire int64) error{
	if len(keys) != len(fields){
		return errors.New("map keys len not equal fields len")
	}
	s.mapMutex.Lock()
	defer s.mapMutex.Unlock()

	m, ok := s.mapCaches[hmKey]
	if !ok{
		m = MapValue{Data: make(map[string]string), Key:hmKey, Expire:ExpireForever}
	}

	if expire == ExpireForever{
		m.Expire = ExpireForever
	}else{
		m.Expire = time.Now().UnixNano() + expire*int64(time.Second)
	}

	//存放变化的值
	temp := MapValue{Data: make(map[string]string), Key:hmKey, Expire:m.Expire}
	for i:=0; i<len(keys); i++ {
		m.Data[keys[i]] = fields[i]
		temp.Data[keys[i]] = fields[i]
	}

	s.mapCaches[hmKey] = m

	op := persistentMapOp{item: m, opType:Add}
	s.persistentMapChan <- op

	//只推送变化的值
	if s.opFunction != nil{
		s.opFunction(Add, &temp)
	}

	return nil
}

func (s *Cache) HMGet(hmKey string) (string, error){
	s.mapMutex.Lock()
	defer s.mapMutex.Unlock()
	m, ok := s.mapCaches[hmKey]
	if !ok {
		str := fmt.Sprintf("not have key:%s map", hmKey)
		return "", errors.New(str)
	}
	d, err := json.MarshalIndent(m.Data, "", "")
	return string(d), err
}


func (s *Cache) HMGetMember(hmKey string, fieldKey string) (string, error){
	s.mapMutex.Lock()
	defer s.mapMutex.Unlock()
	m, ok := s.mapCaches[hmKey]
	if !ok {
		str := fmt.Sprintf("not have key:%s map", hmKey)
		return "", errors.New(str)
	}

	d, ok := m.Data[fieldKey]
	if ok {
		return d, nil
	}else{
		str := fmt.Sprintf("%s map not have field: %s", hmKey, fieldKey)
		return "", errors.New(str)
	}
}

func (s *Cache) HMDelMember(hmKey string, fieldKey string) error{
	s.mapMutex.Lock()
	defer s.mapMutex.Unlock()

	m, ok := s.mapCaches[hmKey]
	if !ok {
		str := fmt.Sprintf("not have key:%s map", hmKey)
		return errors.New(str)
	}

	v, ok1 := m.Data[fieldKey]
	if ok1 {
		delete(m.Data, fieldKey)
		s.mapCaches[hmKey] = m

		//存放变化的值
		temp := MapValue{Data: make(map[string]string), Key:hmKey, Expire:m.Expire}
		temp.Data[fieldKey] = v

		op := persistentMapOp{item: m, opType:Del}
		s.persistentMapChan <- op

		//只推送变化的值
		if s.opFunction != nil{
			s.opFunction(Del, &temp)
		}
	}

	return nil
}


func (s *Cache) HMDel(hmKey string) error{
	s.mapMutex.Lock()
	defer s.mapMutex.Unlock()
	s.hDel(hmKey)

	return nil
}

func (s *Cache) del(key string) {

	log.Printf("del Key:%s", key)
	_, ok := s.caches[key]
	if ok{
		delete(s.caches, key)
		val := Value{Key: key, Expire: ExpireForever, Data:""}
		op := persistentValueOp{item: val, opType:Del}
		s.persistentChan <- op

		if s.opFunction != nil{
			s.opFunction(Del, &val)
		}
	}
}

func (s *Cache) hDel(key string) {

	log.Printf("hDel Key:%s", key)
	m, ok := s.mapCaches[key]
	if ok {
		delete(s.mapCaches, key)

		op := persistentMapOp{item: m, opType:Del}
		s.persistentMapChan <- op

		if s.opFunction != nil{
			s.opFunction(Del, &m)
		}
	}
}

func (s *Cache) checkExpire() {
	for {
		time.Sleep(time.Duration(s.checkExpireInterval) * time.Second)
		s.mutex.Lock()
		t := time.Now().UnixNano()
		for k, v := range s.caches  {
			if v.Expire != ExpireForever && v.Expire <= t{
				s.del(k)
			}
		}
		s.mutex.Unlock()

		time.Sleep(time.Second)

		s.mapMutex.Lock()
		t1 := time.Now().UnixNano()
		for k, v := range s.mapCaches  {
			if v.Expire != ExpireForever && v.Expire <= t1{
				s.hDel(k)
			}
		}
		s.mapMutex.Unlock()
	}
}

func (s *Cache) persistent()  {
	for{
		select {
			case op := <-s.persistentChan:
				v := op.item
				if op.opType == Add {
					s.saveDataBaseKV(v.Key, v)
				}else if op.opType == Del{
					s.delDatabaseKV(v.Key)
				}
			case op := <-s.persistentMapChan:
				v := op.item
				if op.opType == Add {
					s.hSaveDatabaseKV(v.Key, v)
				}else if op.opType == Del{
					s.hDelDatabase(v.Key)
				}
			}
	}
}

func (s *Cache) saveDataBaseKV(key string, v Value) {
	b := encode(v)

	fullPath := filepath.Join(ValueDBPath, key)
	path, _ := filepath.Split(fullPath)

	createDir(path)

	err := ioutil.WriteFile(fullPath, b, os.ModePerm)
	if err != nil{
		log.Printf("saveDataBaseKV error:%s", err.Error())
	}
}

func (s *Cache) hSaveDatabaseKV(key string, v MapValue) {
	b := encodeHM(v)

	fullPath := filepath.Join(MapDBPath, key)
	path, _ := filepath.Split(fullPath)

	createDir(path)

	err := ioutil.WriteFile(fullPath, b, os.ModePerm)
	if err != nil{
		log.Printf("saveDataBaseKV error:%s", err.Error())
	}
}

func (s *Cache) delDatabaseKV(key string)  {
	fullPath := filepath.Join(ValueDBPath, key)
	os.Remove(fullPath)
}

func (s *Cache) hDelDatabase(key string)  {
	fullPath := filepath.Join(MapDBPath, key)
	os.Remove(fullPath)
}

func (s *Cache) ValueCaches() map[string]Value {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.caches
}

func (s *Cache) MapCaches() map[string]MapValue {
	s.mapMutex.Lock()
	defer s.mapMutex.Unlock()
	return s.mapCaches
}

