package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"text/template"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/syhlion/greq"
	"github.com/syhlion/gua/delayquene"
	"github.com/syhlion/gua/luacore"
	guaproto "github.com/syhlion/gua/proto"
	requestwork "github.com/syhlion/requestwork.v2"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func cmdInit(c *cli.Context) (conf *Config) {
	var err error
	logger = logrus.New()
	if c.String("env-file") != "" {
		envfile := c.String("env-file")
		//flag.Parse()
		err := godotenv.Load(envfile)
		if err != nil {
			logger.Fatal(err)
		}
	}
	conf = &Config{}

	conf.GrpcListen = os.Getenv("GRPC_LISTEN")
	if conf.GrpcListen == "" {
		logger.Fatal("empty env GRPC_LISTEN")
	}
	conf.HttpListen = os.Getenv("HTTP_LISTEN")
	if conf.HttpListen == "" {
		logger.Fatal("empty env HTTP_LISTEN")
	}
	conf.HttpFuncListen = os.Getenv("HTTP_FUNC_LISTEN")
	if conf.HttpFuncListen == "" {
		logger.Fatal("empty env HTTP_FUNC_LISTEN")
	}
	conf.Hostname, err = GetHostname()
	if err != nil {
		logger.Fatal(err)
	}
	conf.ExternalIp, err = GetExternalIP()
	if err != nil {
		logger.Fatal(err)
	}
	conf.Mac, err = GetMacAddr()
	if err != nil {
		logger.Fatal(err)
	}
	conf.JobReplyHook = os.Getenv("JOB_REPLY_HOOK")
	conf.RedisForApiAddr = os.Getenv("REDIS_FOR_API_ADDR")
	if conf.RedisForApiAddr == "" {
		logger.Fatal("empty env REDIS_FOR_API_ADDR")
	}
	conf.RedisForApiDBNo, err = strconv.Atoi(os.Getenv("REDIS_FOR_API_DB_NO"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_API_DB_NO")
	}
	conf.RedisForApiMaxIdle, err = strconv.Atoi(os.Getenv("REDIS_FOR_API_MAX_IDLE"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_API_MAX_IDLE")
	}
	conf.RedisForApiMaxConn, err = strconv.Atoi(os.Getenv("REDIS_FOR_API_MAX_CONN"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_API_ADDR")
	}

	conf.RedisForReadyAddr = os.Getenv("REDIS_FOR_READY_ADDR")
	if conf.RedisForReadyAddr == "" {
		logger.Fatal("empty env REDIS_FOR_READY_ADDR")
	}
	conf.RedisForReadyDBNo, err = strconv.Atoi(os.Getenv("REDIS_FOR_READY_DB_NO"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_READY_DB_NO")
	}
	conf.RedisForReadyMaxIdle, err = strconv.Atoi(os.Getenv("REDIS_FOR_READY_MAX_IDLE"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_READY_MAX_IDLE")
	}
	conf.RedisForReadyMaxConn, err = strconv.Atoi(os.Getenv("REDIS_FOR_READY_MAX_CONN"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_READY_MAX_CONN")
	}

	conf.RedisForDelayQueneAddr = os.Getenv("REDIS_FOR_DELAY_QUENE_ADDR")
	if conf.RedisForDelayQueneAddr == "" {
		logger.Fatal("empty env REDIS_FOR_DELAY_QUENE_ADDR")
	}
	conf.RedisForDelayQueneDBNo, err = strconv.Atoi(os.Getenv("REDIS_FOR_DELAY_QUENE_DB_NO"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_DELAY_QUENE_DB_NO")
	}
	conf.RedisForDelayQueneMaxIdle, err = strconv.Atoi(os.Getenv("REDIS_FOR_DELAY_QUENE_MAX_IDLE"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_DELAY_QUENE_MAX_IDLE")
	}
	conf.RedisForDelayQueneMaxConn, err = strconv.Atoi(os.Getenv("REDIS_FOR_DELAY_QUENE_MAX_CONN"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_DELAY_QUENE_MAX_CONN")
	}

	conf.RedisForGroupAddr = os.Getenv("REDIS_FOR_GROUP_ADDR")
	if conf.RedisForGroupAddr == "" {
		logger.Fatal("empty env REDIS_FOR_GROUP_ADDR")
	}
	conf.RedisForGroupDBNo, err = strconv.Atoi(os.Getenv("REDIS_FOR_GROUP_DB_NO"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_GROUP_DB_NO")
	}
	conf.RedisForGroupMaxIdle, err = strconv.Atoi(os.Getenv("REDIS_FOR_GROUP_MAX_IDLE"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_GROUP_MAX_IDLE")
	}
	conf.RedisForGroupMaxConn, err = strconv.Atoi(os.Getenv("REDIS_FOR_GROUP_MAX_CONN"))
	if err != nil {
		logger.Fatal("empty env REDIS_FOR_GROUP_MAX_CONN")
	}
	conf.MachineCode = os.Getenv("MACHINE_CODE")
	if conf.MachineCode == "" {
		logger.Fatal("empty env MACHINE_CODE")
	}
	conf.CompileDate = compileDate
	conf.Version = version
	conf.StartTime = time.Now()
	return
}

func start(c *cli.Context) {

	conf := cmdInit(c)
	//init ready quene redis pool
	apiRedis := redis.NewPool(func() (redis.Conn, error) {
		c, err := redis.Dial("tcp", conf.RedisForApiAddr)
		if err != nil {
			return nil, err
		}
		_, err = c.Do("SELECT", conf.RedisForApiDBNo)
		if err != nil {
			c.Close()
			return nil, err
		}
		return c, nil
	}, 10)
	apiRedis.MaxIdle = conf.RedisForApiMaxIdle
	apiRedis.MaxActive = conf.RedisForApiMaxConn
	func() (err error) {
		apiconn := apiRedis.Get()
		defer apiconn.Close()

		// Test the connection
		_, err = apiconn.Do("PING")
		if err != nil {
			logger.Fatal(err)
			return
		}
		return
	}()
	groupRedis := redis.NewPool(func() (redis.Conn, error) {
		c, err := redis.Dial("tcp", conf.RedisForGroupAddr)
		if err != nil {
			return nil, err
		}
		_, err = c.Do("SELECT", conf.RedisForGroupDBNo)
		if err != nil {
			c.Close()
			return nil, err
		}
		return c, nil
	}, 10)
	groupRedis.MaxIdle = conf.RedisForGroupMaxIdle
	groupRedis.MaxActive = conf.RedisForGroupMaxConn
	func() (err error) {
		groupconn := groupRedis.Get()
		defer groupconn.Close()

		// Test the connection
		_, err = groupconn.Do("PING")
		if err != nil {
			logger.Fatal(err)
			return
		}
		return
	}()
	delayRedis := redis.NewPool(func() (redis.Conn, error) {
		c, err := redis.Dial("tcp", conf.RedisForDelayQueneAddr)
		if err != nil {
			return nil, err
		}
		_, err = c.Do("SELECT", conf.RedisForDelayQueneDBNo)
		if err != nil {
			c.Close()
			return nil, err
		}
		return c, nil
	}, 10)
	delayRedis.MaxIdle = conf.RedisForDelayQueneMaxIdle
	delayRedis.MaxActive = conf.RedisForDelayQueneMaxConn
	func() (err error) {
		delayconn := delayRedis.Get()
		defer delayconn.Close()

		// Test the connection
		_, err = delayconn.Do("PING")
		if err != nil {
			log.Fatal(err)
			return
		}
		return
	}()
	readyRedis := redis.NewPool(func() (redis.Conn, error) {
		c, err := redis.Dial("tcp", conf.RedisForReadyAddr)
		if err != nil {
			return nil, err
		}
		_, err = c.Do("SELECT", conf.RedisForReadyDBNo)
		if err != nil {
			c.Close()
			return nil, err
		}
		return c, nil
	}, 10)
	readyRedis.MaxIdle = conf.RedisForReadyMaxIdle
	readyRedis.MaxActive = conf.RedisForReadyMaxConn
	func() (err error) {
		readyconn := readyRedis.Get()
		defer readyconn.Close()

		// Test the connection
		_, err = readyconn.Do("PING")
		if err != nil {
			log.Fatal(err)
			return
		}
		return
	}()
	dconf := &delayquene.Config{
		MachineIp:   conf.ExternalIp,
		MachineMac:  conf.Mac,
		MachineHost: conf.Hostname,
		JobReplyUrl: conf.JobReplyHook,
		Logger:      logger,
	}
	quene, err := delayquene.New(dconf, groupRedis, readyRedis, delayRedis)
	if err != nil {
		logger.Fatal(err)
		return
	}
	lpool := luacore.New()
	//server
	apiListener, err := net.Listen("tcp", conf.GrpcListen)
	if err != nil {
		log.Println(err)
		return
	}

	work := requestwork.New(100)
	client := greq.New(work, 60*time.Second, true)

	group := &Group{
		apiRedis:   apiRedis,
		groupRedis: groupRedis,
		delayRedis: delayRedis,
	}

	// 註冊 grpc
	sr := &Gua{
		config:     conf,
		httpClient: client,
		quene:      quene,
		rpool:      groupRedis,
	}

	grpc := grpc.NewServer()
	guaproto.RegisterGuaServer(grpc, sr)

	reflection.Register(grpc)
	httpErr := make(chan error)
	httpFuncErr := make(chan error)
	grpcErr := make(chan error)
	httpApiListener, err := net.Listen("tcp", conf.HttpListen)
	r := mux.NewRouter()
	r.HandleFunc("/register/group", RegisterGroup(quene, conf)).Methods("POST")
	r.HandleFunc("/add/job", AddJob(quene, conf)).Methods("POST")
	r.HandleFunc("/add/func", AddFunc(group, apiRedis, lpool)).Methods("POST")
	r.HandleFunc("/delete/job", RemoveJob(quene)).Methods("POST")
	r.HandleFunc("/pause/job", PauseJob(quene)).Methods("POST")
	r.HandleFunc("/active/job", ActiveJob(quene)).Methods("POST")
	r.HandleFunc("/{group_name}/job/list", GetJobList(quene)).Methods("GET")
	r.HandleFunc("/group/list", GetGroupList(quene)).Methods("GET")
	r.HandleFunc("/{group_name}/group/info", GroupInfo(quene)).Methods("GET")
	r.HandleFunc("/{group_name}/node/list", GetNodeList(quene)).Methods("GET")
	//r.HandleFunc("/edit", EditJob(quene)).Methods("POST")
	//r.HandleFunc("/luatest", LuaEntrance(apiRedis, lpool))
	server := http.Server{
		ReadTimeout: 3 * time.Second,
		Handler:     r,
	}
	go func() {
		httpErr <- server.Serve(httpApiListener)
	}()

	httpFuncListener, err := net.Listen("tcp", conf.HttpFuncListen)
	rFunc := mux.NewRouter()
	rFunc.HandleFunc("/{group_name}/{func_name}", LuaEntrance(group, apiRedis, lpool))
	serverFunc := http.Server{
		ReadTimeout: 3 * time.Second,
		Handler:     rFunc,
	}
	go func() {
		httpFuncErr <- serverFunc.Serve(httpFuncListener)
	}()

	go func() {
		grpcErr <- grpc.Serve(apiListener)
	}()
	shutdow_observer := make(chan os.Signal, 1)
	t := template.Must(template.New("gua start msg").Parse(guaMsgFormat))
	t.Execute(os.Stdout, conf)
	signal.Notify(shutdow_observer, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	select {
	case <-shutdow_observer:
		logger.Info("receive signal")
	case err := <-grpcErr:
		logger.Error(err)
	case err := <-httpFuncErr:
		logger.Error(err)
	case err := <-httpErr:
		logger.Error(err)
	}
	return
}
