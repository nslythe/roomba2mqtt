package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
)

type GigyaLoginResponse struct {
	UIDSignature       string `json:"UIDSignature"`
	SignatureTimestamp string `json:"signatureTimestamp"`
	UID                string `json:"UID"`
}

type LoginBody struct {
	AppId                string `json:"app_id"`
	AssumeRobotOwnership int    `json:"assume_robot_ownership"`
	Gigya                struct {
		Signature string `json:"signature"`
		Timestamp string `json:"timestamp"`
		Uid       string `json:"uid"`
	} `json:"gigya"`
}

type Robot struct {
	Password    string `json:"password"`
	Sku         string `json:"sku"`
	SoftwareVer string `json:"softwareVer"`
	Name        string `json:"name"`
}

type RoombaLoginResponse struct {
	Credentials struct {
		AccessKeyId  string `json:"AccessKeyId"`
		SecretKey    string `json:"SecretKey"`
		SessionToken string `json:"SessionToken"`
		Expiration   string `json:"Expiration"`
		CognitoId    string `json:"CognitoId"`
	} `josn:"credentials"`
	robots map[string]Robot
}

type VacuumCredential struct {
	Name     string
	User     string
	Password string
}

func GetCredential(email string, poassword string) map[string]Robot {
	os.MkdirAll("data", 0755)
	gigya_login_response := GetSessionToken(email, poassword, "data/session.json")
	roomba_login_response := RoombaLogin(gigya_login_response, "data/roomba_session.json")
	return roomba_login_response.robots
}

func RoombaLogin(gigya_login_response GigyaLoginResponse, session_file string) RoombaLoginResponse {
	var data []byte
	data, err := ioutil.ReadFile(session_file)
	if err != nil {
		login_body := LoginBody{
			AppId:                "ANDROID-C7FB240E-DF34-42D7-AE4E-A8C17079A294",
			AssumeRobotOwnership: 0,
		}
		login_body.Gigya.Signature = gigya_login_response.UIDSignature
		login_body.Gigya.Uid = gigya_login_response.UID
		login_body.Gigya.Timestamp = gigya_login_response.SignatureTimestamp

		json_body, _ := json.Marshal(login_body)
		reader := bytes.NewReader(json_body)

		req, _ := http.NewRequest("POST", "https://unauth2.prod.iot.irobotapi.com/v2/login", reader)

		res, _ := http.DefaultClient.Do(req)
		data, _ = ioutil.ReadAll(res.Body)
		ioutil.WriteFile(session_file, data, 0644)
	}

	roomba_login_response := RoombaLoginResponse{}
	json.Unmarshal(data, &roomba_login_response)

	return roomba_login_response
}

func GetSessionToken(email string, password string, session_file string) GigyaLoginResponse {
	var data []byte
	data, err := ioutil.ReadFile(session_file)
	if err != nil {

		req, _ := http.NewRequest("POST", "https://accounts.us1.gigya.com/accounts.login", nil)

		q := req.URL.Query()
		q.Add("apiKey", "3_rWtvxmUKwgOzu3AUPTMLnM46lj-LxURGflmu5PcE_sGptTbD-wMeshVbLvYpq01K")
		q.Add("targetenv", "mobile")
		q.Add("loginID", email)
		q.Add("password", password)
		q.Add("format", "json")
		q.Add("targetEnv", "mobile")
		req.URL.RawQuery = q.Encode()

		res, _ := http.DefaultClient.Do(req)

		data, _ = ioutil.ReadAll(res.Body)
		ioutil.WriteFile(session_file, data, 0644)
	}

	gigya_login_response := GigyaLoginResponse{}
	json.Unmarshal(data, &gigya_login_response)

	return gigya_login_response
}
