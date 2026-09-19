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
