package keywords

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/url"
 "strings"
)

const businessUpdatesPostPath="/v3/business_data/google/my_business_updates/task_post"
type BusinessUpdatesTaskInput struct {Keyword,Coordinate string;LocationCode int;LanguageCode string;Depth int}
type BusinessUpdatesTaskProvider interface {StartBusinessUpdatesTask(context.Context,string,BusinessUpdatesTaskInput)(string,error);PollBusinessUpdatesTask(context.Context,string)(bool,map[string]any,error)}
func(p DataForSEOProvider)StartBusinessUpdatesTask(ctx context.Context,org string,in BusinessUpdatesTaskInput)(string,error){
 body:=map[string]any{"keyword":in.Keyword,"language_code":in.LanguageCode,"depth":in.Depth,"priority":2};if in.Coordinate!=""{body["location_coordinate"]=in.Coordinate}else{body["location_code"]=in.LocationCode}
 payload,err:=json.Marshal([]any{body});if err!=nil{return "",err};raw,err:=p.Client.Do(ctx,org,http.MethodPost,businessUpdatesPostPath,payload,false);if err!=nil{return "",err}
 var env struct{StatusCode int `json:"status_code"`;StatusMessage string `json:"status_message"`;Tasks []struct{ID string `json:"id"`;StatusCode int `json:"status_code"`;StatusMessage string `json:"status_message"`} `json:"tasks"`}
 if err=json.Unmarshal(raw,&env);err!=nil{return "",err};if env.StatusCode!=20000{return "",fmt.Errorf("DataForSEO task_post failed: %s",env.StatusMessage)};if len(env.Tasks)==0{return "",errors.New("DataForSEO did not return an update task")};t:=env.Tasks[0];if t.StatusCode!=20100||t.ID==""{return "",fmt.Errorf("DataForSEO update task was not created: %s",t.StatusMessage)};return t.ID,nil
}
func(p DataForSEOProvider)PollBusinessUpdatesTask(ctx context.Context,id string)(bool,map[string]any,error){
 raw,err:=p.Client.DoUnmetered(ctx,http.MethodGet,"/v3/business_data/google/my_business_updates/task_get/"+url.PathEscape(id),nil,true);if err!=nil{return false,nil,err}
 var env struct{StatusCode int `json:"status_code"`;StatusMessage string `json:"status_message"`;Tasks []struct{StatusCode int `json:"status_code"`;StatusMessage string `json:"status_message"`;Result []map[string]any `json:"result"`} `json:"tasks"`}
 if err=json.Unmarshal(raw,&env);err!=nil{return false,nil,err};if env.StatusCode!=20000{return false,nil,fmt.Errorf("DataForSEO task_get failed: %s",env.StatusMessage)};if len(env.Tasks)==0{return false,nil,errors.New("DataForSEO update task was not found")}
 t:=env.Tasks[0];switch t.StatusCode{case 20100,40601,40602:return true,nil,nil}
 if t.StatusCode!=20000{if t.StatusCode==40501&&strings.Contains(strings.ToLower(t.StatusMessage),"no search results"){return false,nil,nil};return false,nil,fmt.Errorf("DataForSEO update task failed: %s",t.StatusMessage)}
 if len(t.Result)==0{return false,nil,nil};return false,t.Result[0],nil
}
