package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
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

type persistentListOp struct {
	item   ListValue
	opType OpType
}

type Cache struct{
	caches     map[string]Value
	mapCaches  map[string]MapValue
	listCaches map[string]ListValue

	checkExpireInterval int
	persistentChan chan persistentValueOp
	persistentMapChan chan persistentMapOp
	persistentListChan chan persistentListOp

	opFunction func(OpType, DataString, DataString)
	mutex      sync.RWMutex
	mapMutex   sync.RWMutex
	listMutex  sync.RWMutex
}

const ExpireForever = 0
var DefaultDBPath = "db"
var ValueDBPath = path.Join(DefaultDBPath, "Value")
var MapDBPath = path.Join(DefaultDBPath, "map")
var ListDBPath = path.Join(DefaultDBPath, "list")

func NewCache(checkExpireInterval int) *Cache {
	 c := Cache{
	 	caches:              make(map[string]Value),
	 	mapCaches:           make(map[string]MapValue),
	 	listCaches:          make(map[string]ListValue),
	 	checkExpireInterval: checkExpireInterval,
	 	persistentChan:      make(chan persistentValueOp),
	 	persistentMapChan:   make(chan persistentMapOp),
	 	persistentListChan:  make(chan persistentListOp),
	 	opFunction:          nil,
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
		 	 v := decodeValue(data)
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

	//list类型
	filepath.Walk(ListDBPath, func(path string, f os.FileInfo, err error) error {
		if f == nil {
			return err
		}
		if f.IsDir() {
			return nil
		}

		if data, err := ioutil.ReadFile(path); err != nil {
			log.Println(err)
		}else {
			v := decodeList(data)
			s.listCaches[v.Key] = v
		}
		return nil
	})

	log.Printf("load db finish, %d Key-cacheValue ", len(s.caches)+len(s.mapCaches)+len(s.listCaches))
}

func (s*Cache) SetOnOP(opFunc func(OpType, DataString, DataString)) {
	s.opFunction = opFunc
}


/*
value
*/
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
			s.opFunction(Add, &old, &val)
		}
	}else{
		needUpdate = true
		e := time.Now().UnixNano() + expire*int64(time.Second)
		val = Value{Key: key, Data: v, Expire:e}
		s.caches[key] = val
		if s.opFunction != nil{
			s.opFunction(Add, &old, &val)
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

func (s *Cache) del(key string) {

	log.Printf("del Key:%s", key)
	old, ok := s.caches[key]
	if ok{
		delete(s.caches, key)
		val := Value{Key: key, Expire: ExpireForever, Data:""}
		op := persistentValueOp{item: val, opType:Del}
		s.persistentChan <- op

		if s.opFunction != nil{
			s.opFunction(Del, &old, nil)
		}
	}
}

func (s *Cache) saveDataBaseKV(key string, v Value) {
	b := encodeValue(v)

	fullPath := filepath.Join(ValueDBPath, key)
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


func (s *Cache) ValueCaches() map[string]Value {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.caches
}



/*
map
*/
func (s *Cache) HMPut(hmKey string, keys [] string,  fields [] string, expire int64) error{
	if len(keys) != len(fields){
		return errors.New("map keys len not equal fields len")
	}
	s.mapMutex.Lock()
	defer s.mapMutex.Unlock()

	m, ok := s.mapCaches[hmKey]
	old := m
	if !ok{
		m = MapValue{Data: make(map[string]string), Key:hmKey, Expire:ExpireForever}
	}

	if expire == ExpireForever{
		m.Expire = ExpireForever
	}else{
		m.Expire = time.Now().UnixNano() + expire*int64(time.Second)
	}

	for i:=0; i<len(keys); i++ {
		m.Data[keys[i]] = fields[i]
	}

	s.mapCaches[hmKey] = m

	op := persistentMapOp{item: m, opType:Add}
	s.persistentMapChan <- op

	//只推送变化的值
	if s.opFunction != nil{
		s.opFunction(Add, &old, &m)
	}

	return nil
}

func (s *Cache) HMGet(hmKey string) (string, error){
	s.mapMutex.RLock()
	defer s.mapMutex.RUnlock()
	m, ok := s.mapCaches[hmKey]
	if !ok {
		str := fmt.Sprintf("not have key:%s map", hmKey)
		return "", errors.New(str)
	}
	d, err := json.MarshalIndent(m.Data, "", "")
	return string(d), err
}


func (s *Cache) HMGetMember(hmKey string, fieldKey string) (string, error){
	s.mapMutex.RLock()
	defer s.mapMutex.RUnlock()
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
	old := m
	if !ok {
		str := fmt.Sprintf("not have key:%s map", hmKey)
		return errors.New(str)
	}

	_, ok1 := m.Data[fieldKey]
	if ok1 {
		delete(m.Data, fieldKey)
		s.mapCaches[hmKey] = m

		op := persistentMapOp{item: m, opType:Del}
		s.persistentMapChan <- op

		//只推送变化的值
		if s.opFunction != nil{
			s.opFunction(Del, &old, &m)
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

func (s *Cache) hDel(key string) {

	log.Printf("hDel Key:%s", key)
	m, ok := s.mapCaches[key]
	old := m
	if ok {
		delete(s.mapCaches, key)
		m.Data = nil
		op := persistentMapOp{item: old, opType:Del}
		s.persistentMapChan <- op

		if s.opFunction != nil{
			s.opFunction(Del, &old, &m)
		}
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

func (s *Cache) hDelDatabase(key string)  {
	fullPath := filepath.Join(MapDBPath, key)
	os.Remove(fullPath)
}

func (s *Cache) MapCaches() map[string]MapValue {
	s.mapMutex.Lock()
	defer s.mapMutex.Unlock()
	return s.mapCaches
}


/*
list
*/
func (s *Cache) LPut(key string, value []string, expire int64) error{
	s.listMutex.Lock()
	defer s.listMutex.Unlock()
	arr, ok := s.listCaches[key]
	old := arr
	if !ok{
		arr = ListValue{Expire:expire, Key:key}
	}
	arr.Expire = expire
	arr.Data = append(arr.Data, value...)
	s.listCaches[key] = arr

	op := persistentListOp{item: arr, opType:Add}
	s.persistentListChan <- op

	if s.opFunction != nil{
		s.opFunction(Add, &old, &arr)
	}

	return nil
}

func (s *Cache) LDel(key string) error{
	s.listMutex.Lock()
	defer s.listMutex.Unlock()
	s.del(key)
	return nil
}

func (s *Cache) LGet(key string) (string, error){
	s.listMutex.RLock()
	defer s.listMutex.RUnlock()
	m, ok := s.listCaches[key]
	if !ok {
		str := fmt.Sprintf("not have key:%s list", key)
		return "", errors.New(str)
	}

	d, err := json.MarshalIndent(m.Data, "", "")
	return string(d), err
}

func (s *Cache) LGetRange(key string, beg int, end int) (string, error){

	if beg > end{
		str := fmt.Sprintf("list: %s begin index > end index ", key)
		return "", errors.New(str)
	}

	s.listMutex.RLock()
	defer s.listMutex.RUnlock()

	m, ok := s.listCaches[key]
	if !ok {
		str := fmt.Sprintf("not have key:%s list", key)
		return "", errors.New(str)
	}

	l :=len(m.Data)
	if beg >= l{
		str := fmt.Sprintf("list: %s out off range ", key)
		return "", errors.New(str)
	}

	min := int(math.Min(float64(end), float64(l)))
	arr := m.Data[beg:min]

	d, err := json.MarshalIndent(arr, "", "")
	return string(d), err
}

func (s *Cache) LDelRange(key string, beg int, end int)  error{

	if beg > end{
		str := fmt.Sprintf("list: %s begin index > end index ", key)
		return errors.New(str)
	}

	s.listMutex.RLock()
	defer s.listMutex.RUnlock()

	m, ok := s.listCaches[key]
	old := m
	if !ok {
		str := fmt.Sprintf("not have key:%s list", key)
		return errors.New(str)
	}

	l :=len(m.Data)
	if beg >= l{
		str := fmt.Sprintf("list: %s out off range ", key)
		return errors.New(str)
	}

	min := int(math.Min(float64(end), float64(l)))
	b := m.Data[0:beg]
	e := m.Data[min:]

	m.Data = append(b, e...)
	s.listCaches[key] = m

	op := persistentListOp{item: m, opType:Del}
	s.persistentListChan <- op

	//变化后的list
	if s.opFunction != nil{
		s.opFunction(Del, &old, &m)
	}

	return  nil
}

func (s *Cache) lDel(key string) {

	log.Printf("lDel Key:%s", key)
	m, ok := s.listCaches[key]
	old := m
	if ok {
		delete(s.listCaches, key)

		m.Data = nil
		op := persistentListOp{item: m, opType:Del}
		s.persistentListChan <- op

		if s.opFunction != nil{
			s.opFunction(Del, &old, &m)
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
					if v.Data == nil{
						s.hDelDatabase(v.Key)
					}else{
						s.hSaveDatabaseKV(v.Key, v)
					}
				}
			case op := <-s.persistentListChan:
				v := op.item
				if op.opType == Add {
					s.lSaveDataBaseKV(v.Key, v)
				}else if op.opType == Del{
					if v.Data == nil{
						s.lDelDatabaseKV(v.Key)
					}else{
						s.lSaveDataBaseKV(v.Key, v)
					}
				}
			}
	}
}

func (s *Cache) lSaveDataBaseKV(key string, v ListValue) {
	b := encodeList(v)

	fullPath := filepath.Join(ValueDBPath, key)
	path, _ := filepath.Split(fullPath)

	createDir(path)

	err := ioutil.WriteFile(fullPath, b, os.ModePerm)
	if err != nil{
		log.Printf("saveDataBaseKV error:%s", err.Error())
	}
}

func (s *Cache) lDelDatabaseKV(key string)  {
	fullPath := filepath.Join(ListDBPath, key)
	os.Remove(fullPath)
}


func (s *Cache) ListCaches() map[string]ListValue {
	s.listMutex.Lock()
	defer s.listMutex.Unlock()
	return s.listCaches
}


