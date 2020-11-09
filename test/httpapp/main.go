package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

var (
	myerr = errors.New("this is not an error")
)

type User struct {
	ID       int64  `json:"id"`
	UserName string `json:"username"`
	NickName string `json:"nickname"`
	PassWord string `json:"password"`
	Status   int    `json:"status"`
}

func main() {
	mysqlService := getMysqlService2()
	dataSource := fmt.Sprintf("root:q755100802@tcp(%s)/user_db", mysqlService)
	db, err := sql.Open("mysql", dataSource)
	if err != nil {
		panic(err)
	}

	fmt.Println("why?")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		//query := "SELECT id, uid, user_name, nick_name FROM user_base_info_tab_00000000 LIMIT 3"
		query := "Select * from user_db where username='test'"
		rows, err := db.Query(query)

		if err != nil {
			fmt.Fprintln(w, err.Error())
			return
		}
		bt, _ := json.Marshal(scan(rows))
		fmt.Fprintln(w, string(bt))
	})
	http.ListenAndServe("127.0.0.1:9100", nil)
}

func scan(rows *sql.Rows) []User {
	if rows == nil {
		return nil
	}
	var users []User
	for rows.Next() {
		var user User
		err := rows.Scan(&user.ID, &user.UserName, &user.NickName, &user.PassWord, &user.Status)
		if err != nil {
			fmt.Println("error: ", err.Error())
			continue
		}
		users = append(users, user)
	}
	return users
}

//获取mysql service的host和port
func getMysqlService() string {
	mysqlServiceHost := os.Getenv("MYSQL_SERVICE_SERVICE_HOST")
	mysqlServicePort := os.Getenv("MYSQL_SERVICE_SERVICE_PORT")
	return mysqlServiceHost + ":" + mysqlServicePort
}

func getMysqlService2() string {
	return "127.0.0.1:3306"
}
