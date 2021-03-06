# lightkv 轻量化key-value缓存服务
- 支持字符串key-value、 key-map、key-list、key-set存储
- 可持久化到本地
- 提供api访问和grpc访问接口
- 简单易用
## 启动server
```bash
go run main/server.go  
```
  
  
- 会启动一个api服务(http://localhost:9981) 和一个rpc服务(9980端口)

- api 提供的方法有 put、del、get、hput、hget、hgetm、hdelm、hdel、lget、lgetr、lput、ldel、ldelr、sget、sput、sdel、sdelm

### api普通字符串(put、del、get)
- http://localhost:9981/put?key=add1&value=addvalue1 api新增一条kv，key为add1,value为addvalue1，kv不过期 

- http://localhost:9981/put?key=add2&value=addvalue2&expire=100 api新增一条kv，key为add2,value为addvalue2，100秒后kv过期

- http://localhost:9981/del/add2 api删除key为add2的kv

- http://localhost:9981/get/add1 api获取key为add1的kv

### api map(hput、hget、hgetm、hdelm、hdel)
- http://localhost:9981/hput?hmkey=hm1&key=k1&value=v1&key=k2&value=v2 往hm1的map添加两个元素{"k1":"v1","k2":"v2"}

- http://localhost:9981/hget/hm1 获取hm1的map

- http://localhost:9981/hgetm/hm1/k1 获取hm1的map中k1的元素

- http://localhost:9981/hdelm/hm1/k1 删除hm1的map中k1的元素

- http://localhost:9981/hdel/hm1 删除hm1的map

### api list(lget、lgetr、lput、ldel、ldelr)
- http://localhost:9981/lput?key=test&value=a&value=b&value=c&value=d 往test的list添加两个元素{"a":"c","c":"d"}

- http://localhost:9981/lget/test 获取test的list

- http://localhost:9981/lgetr/test?begIndex=1&endIndex=3 获取test的list下表1-3的元素

- http://localhost:9981/ldelr/test?begIndex=1&endIndex=3 删除test的list下表1-3的元素

- http://localhost:9981/ldel/test 删除test的list

### api set(sget、sput、sdel、sdelm)
- http://localhost:9981/sput?key=test&value=a&value=b&value=c&value=d 往test的set添加两个元素{"a":"c","c":"d"}

- http://localhost:9981/sget/test 获取test的set

- http://localhost:9981/sdelm/test?value=a 删除test的set中的a元素

- http://localhost:9981/sdel/test 删除test的set


## 启动测试rpc客户端
```bash
  go run main/client.go  
```
  


## rpc 客户端用法
### 普通字符串 用法
```go 

	c := server.NewClient("127.0.0.1:9980")
	c.Start()
	defer c.Close()

	c.ClearValue()

	//添加kv
	c.Put("test","test_value",0)
	c.Put("test1/tttt","test1_value",0)
	c.Put("test2","test2_value",5)
	c.Put("test3","test3_value",5)

	v := c.Get("test2")
	log.Printf("获取 test2 的值:%s", v)

	//删除kv
	c.Del("test2")
	log.Printf("删除 test2 之后的值:%s", c.Get("test2"))

	//监听key值，发生变化回调通知
	c.WatchKey("watch1", func(k string, beforeV string, afterV string, t kv.OpType) {
		if t == kv.Add {
			log.Printf("监听的 key:%s 新增了, 变化前:%s\n变化后:%s\n", k, beforeV, afterV)
		}else if t == kv.Del {
			log.Printf("监听的 key:%s 删除了, 变化前:%s\n变化后:%s\n", k, beforeV, afterV)
		}
	})

	//key值发生变化回调通知
	c.WatchKey("unwatch", func(k string, beforeV string, afterV string, t kv.OpType) {
		if t == kv.Add {
			log.Printf("监听的 key:%s 新增了, 变化前:%s\n变化后:%s\n", k, beforeV, afterV)
		}else if t == kv.Del {
			log.Printf("监听的 key:%s 删除了, 变化前:%s\n变化后:%s\n", k, beforeV, afterV)
		}
	})

	c.WatchKey("watchdel", func(k string, beforeV string, afterV string, t kv.OpType) {
		if t == kv.Add {
			log.Printf("监听的 key:%s 新增了, 变化前:%s\n变化后:%s\n", k, beforeV, afterV)
		}else if t == kv.Del {
			log.Printf("监听的 key:%s 删除了, 变化前:%s\n变化后:%s\n", k, beforeV, afterV)
		}
	})


	c.Put("unwatch", "this is before unwatch", 0)

	time.Sleep(1*time.Second)
	//取消监听key值的变化
	c.UnWatchKey("unwatch")

	c.Put("watch1", "this is watch1", 0)
	c.Put("unwatch", "this is after unwatch", 0)
	c.Put("watchdel", "this is watchdel", 0)

	log.Printf("获取 watchdel:%s", c.Get("watchdel"))

	c.Del("watchdel")

	log.Printf("获取 unwatch:%s", c.Get("unwatch"))


```

### map 用法

```go

	c := server.NewClient("127.0.0.1:9980")
	c.Start()
	defer c.Close()

	c.ClearMap()

	keys := []string{"k1", "k2", "k3"}
	vals := []string{"v1", "v2", "v3"}

	//新增map
	c.HMPut("hmtest1", keys, vals, 0)

	str := c.HMGet("hmtest1")
	log.Printf("获取hmtest1 map:\n%s", str)

	//删除hmtest1 map 中的k1
	c.HMDelMember("hmtest1", "k1")
	str = c.HMGet("hmtest1")

	log.Printf("hmtest1 map 删除了k1后:\n%s", str)

	c.HMDel("hmtest1")
	log.Printf("删除hmtest1 map后，hmtest1的值:\n%s", c.HMGet("hmtest1"))


	c.HMWatch("hmtest2", "", func(hk string, k string, beforeV string, afterV string, t kv.OpType) {
		if k == ""{
			if t == kv.Add {
				log.Printf("监听的 map key:%s, 新增了, 变化前:%s\n变化后:%s\n", hk, beforeV, afterV)
			}else if t == kv.Del {
				log.Printf("监听的 map key:%s, 删除了, 变化前:%s\n变化后:%s\n", hk, beforeV, afterV)
			}
		}else{
			if t == kv.Add {
				log.Printf("监听的 map key:%s, 元素:%s, 新增了, 变化前:%s\n变化后:%s\n", hk, k, beforeV, afterV)
			}else if t == kv.Del {
				log.Printf("监听的 map key:%s, 元素:%s, 删除了，变化前:%s\n变化后:%s\n", hk, k, beforeV, afterV)
			}
		}
	})

	c.HMWatch("hmtest2", "k1", func(hk string, k string, beforeV string, afterV string, t kv.OpType) {
		if k == ""{
			if t == kv.Add {
				log.Printf("监听的 map key:%s, 新增了, 变化前:%s\n变化后:%s\n", hk, beforeV, afterV)
			}else if t == kv.Del {
				log.Printf("监听的 map key:%s, 删除了, 变化前:%s\n变化后:%s\n", hk, beforeV, afterV)
			}
		}else{
			if t == kv.Add {
				log.Printf("监听的 map key:%s, 元素:%s, 新增了, 变化前:%s\n变化后值为:\n%s", hk, k, beforeV, afterV)
			}else if t == kv.Del {
				log.Printf("监听的 map key:%s, 元素:%s, 删除了, 变化前:%s\n变化后值为:\n%s", hk, k, beforeV, afterV)
			}
		}
	})

	c.HMWatch("hmtest2", "", func(hk string, k string, beforeV string, afterV string, t kv.OpType) {
		if k == ""{
			if t == kv.Add {
				log.Printf("监听的 map key:%s, 新增了, 变化前:%s\n变化后:%s\n", hk, beforeV, afterV)
			}else if t == kv.Del {
				log.Printf("监听的 map key:%s, 删除了, 变化前:%s\n变化后:%s\n", hk, beforeV, afterV)
			}
		}else{
			if t == kv.Add {
				log.Printf("监听的 map key:%s, 元素:%s, 新增了, 变化前:%s\n变化后:%s\n", hk, k, beforeV, afterV)
			}else if t == kv.Del {
				log.Printf("监听的 map key:%s, 元素:%s, 删除了, 变化前:%s\n变化后:%s\n", hk, k, beforeV, afterV)
			}
		}

	})


	log.Printf("新增hmtest2 map")
	//新增hmtest2 map
	c.HMPut("hmtest2", keys, vals, 3)

	log.Printf("获取 hmtest2 map的值:\n%s", c.HMGet("hmtest2"))

	log.Printf("获取hmtest2 map 中k2元素:%s", c.HMGetMember("hmtest2", "k2"))

	//删除hmtest2 中的k2
	log.Printf("删除hmtest2 map 中k1元素")
	c.HMDelMember("hmtest2", "k1")

	c.HMPut("hmtest3", keys, vals, 0)
	log.Printf("获取hmtest2 map 中k1元素:%s", c.HMGetMember("hmtest2", "k1"))

```

### list 用法

```go

	c := server.NewClient("127.0.0.1:9980")
	c.Start()
	defer c.Close()

	c.ClearList()

	c.LPut("testlist", []string{"1","2", "3"}, 10)

	c.LPut("list1", []string{"a1","a2", "a3"}, 0)
	arr, _ := c.LGet("list1")
	log.Printf("获取list1:%v", arr)

	c.LPut("list2", []string{"b1","b2", "b3", "b4", "b5"}, 0)
	arr, _ = c.LGet("list2")
	log.Printf("获取list2:%v", arr)

	arr, _ = c.LGetRange("list2", 0,2)
	log.Printf("获取list2 0-2:元素%v", arr)

	log.Printf("删除list2 1-3位元素")
	c.LDelRange("list2", 1,3)

	arr, _ = c.LGet("list2")
	log.Printf("获取list2:%v", arr)

	c.LWatchKey("watchList", func(k string, beforeV []string, afterV []string, opType kv.OpType) {
		if opType == kv.Add {
			log.Printf("监听 %s 新增了, 变化前:%v\n变化后:%v\n", k, beforeV, afterV)
		}else{
			log.Printf("监听 %s 删除了, 变化前:%v\n变化后:%v\n", k, beforeV, afterV)
		}
	})

	log.Printf("添加watchList")
	c.LPut("watchList", []string{"c1", "c2", "c3", "c4", "c5"}, 0)

	log.Printf("删除watchList的0-2元素")
	c.LDelRange("watchList", 0,2)

	log.Printf("取消监听watchList")
	c.LUnWatchKey("watchList")

	log.Printf("删除watchList")
	c.LDel("watchList")

	arr, _ = c.LGet("watchList")
	log.Printf("获取watchList:%v", arr)


```

### set 用法
```go

	c := server.NewClient("127.0.0.1:9980")
	c.Start()
	defer c.Close()

	c.ClearSet()

	c.SPut("set1", []string{"aset","bset", "cset", "aset"}, 0)

	arr, _:= c.SGet("set1")
	log.Printf("获取set1:%v", arr)

	arr, _= c.SGet("set2")
	log.Printf("在没有存入set2时获取set2:%v", arr)

	c.SPut("set2", []string{"aset2","bset2", "cset2", "aset2"}, 0)

	arr, _= c.SGet("set2")
	log.Printf("存入set2后获取set2:%v", arr)

	log.Printf("删除set2的cset2元素")
	c.SDelMember("set2", "cset2")

	arr, _= c.SGet("set2")
	log.Printf("获取set2:%v", arr)

	log.Printf("删除set1")
	c.SDel("set1")

	arr, _ = c.SGet("set1")
	log.Printf("获取set1:%v", arr)

	c.SWatchKey("setwatch", func(key string, before []string, after []string, opType kv.OpType) {
		if opType == kv.Add {
			log.Printf("监听 %s 新增了，新增前的值为：%v\n新增后的值为:%v\n", key, before, after)
		}else{
			log.Printf("监听 %s 删除了，删除前的值为：%v\n删除后的值为:%v\n", key, before, after)
		}
	})

	c.SPut("setwatch", []string{"setwatch1","setwatch2", "setwatc3", "setwatch4"}, 0)

	c.SDelMember("setwatch", "setwatch1")

	c.SDel("setwatch")


```

## 后续计划
- 支持list、set 结构存储 (已完成)
- 常用的参数支持配置 (已完成)
- 支持配置缓存占用大小，lru算法 (已完成)
- 分布式
- terminal客户端