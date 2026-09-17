package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"openvpn-pannel/internal/api"
	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/dao"
	"openvpn-pannel/internal/models"
	"os"
	"os/signal"
	"syscall"
)

var buildDate string

func getReadableFileSize(fileSize uint64) string {
	var size string
	switch {
	case fileSize >= 1024*1024*1024:
		size = fmt.Sprintf("%.2f GB", float64(fileSize)/(1024*1024*1024))
	case fileSize >= 1024*1024:
		size = fmt.Sprintf("%.2f MB", float64(fileSize)/(1024*1024))
	case fileSize >= 1024:
		size = fmt.Sprintf("%.2f KB", float64(fileSize)/1024)
	case fileSize > 0:
		size = fmt.Sprintf("%d B", fileSize)
	}
	return size
}
func main() {
	configPath := flag.String("config", "config.json", "Path to config file")
	flag.Parse()

	// 获取子命令
	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Usage: server [-config=path] <command>")
		fmt.Println("Commands:")
		fmt.Println("  migratedb      Run database migrations")
		fmt.Println("  run            Start HTTP server")
		fmt.Println("  createuser     Create a new user")
		fmt.Println("  creategroup    Create a new group")
		fmt.Println("  addusertogroup Add user to group")
		fmt.Println("  democonfig     Output a demo config file")
		fmt.Println("  version        Output version")
		os.Exit(1)
	}

	command := args[0]
	if command == "democonfig" {
		demoConfig := config.Config{
			Listen:            ":8080",
			MysqlAddr:         "127.0.0.1:3306",
			MysqlUser:         "your_mysql_user",
			MysqlPass:         "your_mysql_password",
			MysqlDB:           "your_database_name",
			PasswordSalt:      "g8w47diqwhduwgf",
			SessionSecret:     "secret-key-32-byte-long-00001111",
			InternalAPIListen: "127.0.0.1:59003",
			AllowEditResource: false,
		}
		data, err := json.MarshalIndent(demoConfig, "", "  ")
		if err != nil {
			log.Fatalf("Failed to generate demo config: %v", err)
		}
		fmt.Println(string(data))
		os.Exit(0)
	}

	// 加载配置
	cfg, err := config.ReadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	switch command {
	case "migratedb":
		db, err := models.ConnectDB(cfg)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		err = models.MigrateDB(db)
		if err != nil {
			log.Fatalf("Failed to migrate database: %v", err)
		}
		fmt.Println("Database migration completed successfully.")
	case "version":
		// 获取编译日期

		if buildDate == "" {
			buildDate = "unknown"
		}
		fmt.Println(buildDate)
	case "run":
		a := api.NewApp(cfg, buildDate)
		// 捕获退出信号：Ctrl-C 发 SIGINT，procd / systemd 停止服务时发 SIGTERM
		go func() {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)
			<-c
			log.Println("开始关闭所有服务器实例")
			a.StopAllOpenVPNServer()
			os.Exit(0)
		}()
		a.Run()

	case "createuser":
		if len(args) < 4 {
			log.Fatal("Usage: createuser <username> <password> <description>")
		}
		username := args[1]
		password := args[2]
		description := args[3]

		userManager, err := dao.NewDaoManager(cfg)
		if err != nil {
			log.Fatalf("Failed to connect database: %v", err)
		}
		err = userManager.CreateUser(username, password, description,
			models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0)
		if err != nil {
			log.Fatalf("Failed to create user: %v", err)
		}
		fmt.Println("User created successfully.")

	// case "useronline":
	// 	if len(args) < 5 {
	// 		log.Fatal("Usage: useronline <username> <server_id> <real_ip_addr> <virtual_ip_addr>")
	// 	}
	// 	username := args[1]
	// 	_serverID := args[2]
	// 	var serverID uint64
	// 	serverID, err := strconv.ParseUint(_serverID, 10, 64)
	// 	if err != nil {
	// 		log.Fatalf("Failed to parse server_id: %v", err)
	// 	}
	// 	virtualIPAddr := args[4]
	// 	realIPAddr := args[3]
	// 	userManager, err := dao.NewDaoManager(cfg)
	// 	if err != nil {
	// 		log.Fatalf("Failed to connect database: %v", err)
	// 	}
	// 	userManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_ONLINE, realIPAddr, fmt.Sprintf("用户=%s 虚拟IP=%s", username, virtualIPAddr))

	// case "useroffline":
	// 	if len(args) < 5 {
	// 		log.Fatal("Usage: useroffline <username> <server_id> <real_ip_addr> <virtual_ip_addr> <bytes_send(optional)> <bytes_received(optional)>")
	// 	}
	// 	username := args[1]
	// 	_serverID, err := strconv.Atoi(args[2])
	// 	if err != nil {
	// 		log.Fatal("Usage: useroffline <username> <server_id> <real_ip_addr> <virtual_ip_addr> <bytes_send(optional)> <bytes_received(optional)>")
	// 	}
	// 	serverID := uint(_serverID)
	// 	realIPAddr := args[3]
	// 	virtualIPAddr := args[4]
	// 	var bytesSend uint64
	// 	var bytesReceived uint64
	// 	if len(args) >= 7 {
	// 		bytesSend, err = strconv.ParseUint(args[5], 10, 64)
	// 		if err != nil {
	// 			log.Fatal("Usage: useroffline <username> <server_id> <real_ip_addr> <virtual_ip_addr> <bytes_send(optional)> <bytes_received(optional)>")
	// 		}
	// 		bytesReceived, err = strconv.ParseUint(args[6], 10, 64)
	// 		if err != nil {
	// 			log.Fatal("Usage: useroffline <username> <server_id> <real_ip_addr> <virtual_ip_addr> <bytes_send(optional)> <bytes_received(optional)>")
	// 		}
	// 	} else if len(args) == 6 {
	// 		log.Fatal("Usage: useroffline <username> <server_id> <real_ip_addr> <virtual_ip_addr> <bytes_send(optional)> <bytes_received(optional)>")
	// 	}
	// 	daoManager, err := dao.NewDaoManager(cfg)
	// 	daoManager.CreateEvent(serverID, models.SERVER_EVENT_TYPE_CLIENT_OFFLINE, realIPAddr, fmt.Sprintf("用户=%s IP=%s 虚拟IP=%s 发送=%s 接收=%s 发送字节数=%d 接收字节数=%d",
	// 		username, realIPAddr, virtualIPAddr, getReadableFileSize(bytesSend), getReadableFileSize(bytesReceived), bytesSend, bytesReceived))
	// case "deluseracl":
	// 	if len(args) < 4 {
	// 		log.Fatal("Usage: deluseracl <username> <server_id> <real_ip_addr>")
	// 	}
	// 	var serverID uint64

	// 	serverID, err = strconv.ParseUint(args[2], 10, 64)
	// 	if err != nil {
	// 		log.Fatalf("Failed to parse server id: %v", err)
	// 	}

	// 	realIPAddr := args[3]
	// 	username := args[1]
	// 	userManager, err := dao.NewDaoManager(cfg)
	// 	if err != nil {
	// 		log.Fatalf("Failed to create dao manager: %v", err)
	// 	}
	// 	aclListString := ""
	// 	aclList, err := userManager.GetAddedACLByIP(realIPAddr)
	// 	var resultMap map[string]int
	// 	resultMap = make(map[string]int)
	// 	if err == nil {
	// 		for _, acl := range aclList {

	// 			resultMap[fmt.Sprintf("%d#%s", acl.ACLType, acl.ACLValue)] = 114514
	// 		}
	// 	}
	// 	for k := range resultMap {
	// 		fmt.Println(k)
	// 		aclListString = fmt.Sprintf("%s [%s]", aclListString, k)
	// 	}
	// 	userManager.DeleteAddedACLByIP(realIPAddr)
	// 	userManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_DEL_ACL, realIPAddr, fmt.Sprintf("用户=%s 内容=%s", username, aclListString))

	// case "getuseracl":
	// 	if len(args) < 2 {
	// 		log.Fatal("Usage: getuseracl <username> <server_id(optional)> <real_ip_addr(optional)>")
	// 	}
	// 	realIPAddr := ""
	// 	if len(args) == 4 {
	// 		realIPAddr = args[3]
	// 	}
	// 	var serverID uint64
	// 	if len(args) == 4 {
	// 		serverID, err = strconv.ParseUint(args[2], 10, 64)
	// 		if err != nil {
	// 			log.Fatalf("Failed to parse server id: %v", err)
	// 		}
	// 	}
	// 	username := args[1]
	// 	userManager, err := dao.NewDaoManager(cfg)
	// 	if err != nil {
	// 		log.Fatalf("Failed to connect database: %v", err)
	// 	}
	// 	userModel, err := userManager.GetUserByUsername(username)
	// 	if err != nil {
	// 		log.Fatalf("查询用户失败: %v", err)
	// 	}
	// 	groupModelList, err := userManager.ListGroupsForUser(userModel.ID)
	// 	if err != nil {
	// 		log.Fatalf("查询用户的用户组失败: %v", err)
	// 	}
	// 	var resultMap map[string]int
	// 	resultMap = make(map[string]int)
	// 	var addedACLRecordList []*models.AddedServerACLRecord
	// 	addedACLString := ""
	// 	for _, groupModel := range groupModelList {
	// 		groupACLList, err := userManager.ListGroupACL(groupModel.ID)
	// 		if err != nil {
	// 			log.Fatalf("查询用户组ACL失败: %v", err)
	// 		}
	// 		for _, groupACL := range groupACLList {
	// 			addedACLRecordList = append(addedACLRecordList, &models.AddedServerACLRecord{
	// 				RealIPAddr: realIPAddr,
	// 				ACLType:    groupACL.Type,
	// 				ACLValue:   groupACL.Value,
	// 				ServerID:   uint(serverID),
	// 			})
	// 			addedACLString = fmt.Sprintf("%s [%d#%s]", addedACLString, groupACL.Type, groupACL.Value)
	// 			resultMap[fmt.Sprintf("%d#%s", groupACL.Type, groupACL.Value)] = 114514
	// 		}
	// 	}
	// 	if len(addedACLRecordList) != 0 {
	// 		userManager.SaveAddedACL(addedACLRecordList)
	// 	}
	// 	if realIPAddr != "" {
	// 		userManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_ADD_ACL, realIPAddr, fmt.Sprintf("用户=%s ACL=%s", username, addedACLString))
	// 	}
	// 	for k := range resultMap {
	// 		fmt.Println(k)
	// 	}
	// case "authuser", "authuserbase64":
	// 	if len(args) < 4 {
	// 		log.Fatal("Usage: authuser/authuserbase64 <server_id> <username> <password> <real_ip_addr(optional)> <client_cert_name(optional)>")
	// 	}
	// 	realIPAddr := ""
	// 	if len(args) >= 5 {
	// 		realIPAddr = args[4]
	// 	}
	// 	_serverID := args[1]
	// 	serverID, err := strconv.ParseUint(_serverID, 10, 64)
	// 	if err != nil {
	// 		fmt.Print("server_id 需要是整数")
	// 		os.Exit(1)
	// 	}
	// 	var clientCertName string
	// 	if len(args) >= 6 {
	// 		clientCertName = args[5]
	// 	}

	// 	userManager, err := dao.NewDaoManager(cfg)
	// 	if err != nil {
	// 		fmt.Printf("Failed to connect database: %v", err)
	// 		os.Exit(1)

	// 	}
	// 	var username, password string
	// 	if args[0] == "authuserbase64" {
	// 		_username, err1 := base64.StdEncoding.DecodeString(args[2])
	// 		_password, err2 := base64.StdEncoding.DecodeString(args[3])
	// 		if err1 != nil || err2 != nil {
	// 			userManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_FAIL, realIPAddr, fmt.Sprintf("内部错误base64解码失败 证书=%s 用户=%s", clientCertName, username))
	// 			log.Fatal("base64 解码失败")
	// 		}
	// 		username = string(_username)
	// 		password = string(_password)
	// 	} else {
	// 		username = args[2]
	// 		password = args[3]
	// 	}

	// 	userModel, err := userManager.AuthUser(username, password)
	// 	if err != nil {
	// 		fmt.Print("用户名或密码错误")
	// 		userManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_FAIL, realIPAddr, fmt.Sprintf("用户名或密码错误 证书=%s 用户=%s", clientCertName, username))
	// 		os.Exit(1)
	// 	}
	// 	// 检查用户是否有权限
	// 	result, err := userManager.CheckServerPermission(uint(serverID), userModel)
	// 	if err != nil {
	// 		fmt.Print(err.Error())
	// 		userManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_FAIL, realIPAddr, fmt.Sprintf("权限检查失败 证书=%s 用户=%s %s", clientCertName, username, err.Error()))
	// 		os.Exit(1)
	// 	}
	// 	if result {
	// 		fmt.Print("登录成功")
	// 		userManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_SUCCESS, realIPAddr, fmt.Sprintf("登录成功 证书=%s 用户=%s", clientCertName, username))
	// 		os.Exit(0)
	// 	} else {
	// 		fmt.Print("没有权限")
	// 		userManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_FAIL, realIPAddr, fmt.Sprintf("没有权限 证书=%s 用户=%s", clientCertName, username))
	// 		os.Exit(1)
	// 	}

	case "creategroup":
		if len(args) < 2 {
			log.Fatal("Usage: creategroup <groupname>")
		}
		groupname := args[1]

		userManager, err := dao.NewDaoManager(cfg)
		if err != nil {
			log.Fatalf("Failed to connect database: %v", err)
		}
		err = userManager.CreateGroup(groupname, "", 0, 0)
		if err != nil {
			log.Fatalf("Failed to create group: %v", err)
		}
		fmt.Println("Group created successfully.")

	case "addusertogroup":
		if len(args) < 3 {
			log.Fatal("Usage: addusertogroup <username> <groupname>")
		}
		username := args[1]
		groupname := args[2]

		userManager, err := dao.NewDaoManager(cfg)
		if err != nil {
			log.Fatalf("Failed to connect database: %v", err)
		}
		userID, err := userManager.GetUserByUsername(username)
		if err != nil {
			log.Fatalf("Failed to get user: %v", err)
		}
		group, err := userManager.GetGroupByName(groupname)
		if err != nil {
			log.Fatalf("Failed to get group: %v", err)
		}
		err = userManager.AddUserToGroup(userID.ID, group.ID)
		if err != nil {
			log.Fatalf("Failed to add user to group: %v", err)
		}
		fmt.Println("User added to group successfully.")

	default:
		log.Fatalf("Unknown command: %s", command)
	}
}
