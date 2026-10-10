package keywords

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/url"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

type BusinessReviewTaskInput struct {
 Endpoint string
 Keyword string
 CID string
 PlaceID string
 Coordinate string
 LocationCode int
 LanguageCode string
 Depth int
 SortBy string
}
type BusinessReviewTaskProvider interface {
 StartBusinessReviewTask(context.Context,string,BusinessReviewTaskInput)(string,error)
 PollBusinessReviewTask(context.Context,string,string)(bool,map[string]any,error)
}
func (p DataForSEOProvider) StartBusinessReviewTask(ctx context.Context,org string,in BusinessReviewTaskInput)(string,error){
 if in.Endpoint!="reviews"&&in.Endpoint!="extended_reviews"{return "",errors.New("invalid business review task endpoint")}
 body:=map[string]any{"keyword":in.Keyword,"cid":in.CID,"place_id":in.PlaceID,"language_code":in.LanguageCode,"depth":in.Depth,"priority":2}
 if in.Coordinate!=""{body["location_coordinate"]=in.Coordinate}else{body["location_code"]=in.LocationCode}
 if in.Endpoint=="reviews"{body["sort_by"]=in.SortBy}
 raw,err:=json.Marshal([]any{body});if err!=nil{return "",err}
 path:="/v3/business_data/google/"+in.Endpoint+"/task_post"
 response,err:=p.Client.Do(ctx,org,http.MethodPost,path,raw,false);if err!=nil{return "",err}
 var envelope struct{StatusCode int `json:"status_code"`;StatusMessage string `json:"status_message"`;Tasks []struct{ID string `json:"id"`;StatusCode int `json:"status_code"`;StatusMessage string `json:"status_message"`} `json:"tasks"`}
 if err=json.Unmarshal(response,&envelope);err!=nil{return "",fmt.Errorf("decode review task response: %w",err)}
 if envelope.StatusCode!=20000{return "",fmt.Errorf("DataForSEO task_post failed: %s",envelope.StatusMessage)}
 if len(envelope.Tasks)==0{return "",errors.New("DataForSEO did not return a review task")}
 task:=envelope.Tasks[0];if task.StatusCode!=20100||task.ID==""{return "",fmt.Errorf("DataForSEO review task was not created: %s",task.StatusMessage)}
 return task.ID,nil
}
func (p DataForSEOProvider) PollBusinessReviewTask(ctx context.Context,endpoint,taskID string)(bool,map[string]any,error){
 if endpoint!="reviews"&&endpoint!="extended_reviews"{return false,nil,errors.New("invalid business review task endpoint")}
 raw,err:=p.Client.DoUnmetered(ctx,http.MethodGet,"/v3/business_data/google/"+endpoint+"/task_get/"+url.PathEscape(taskID),nil,true);if err!=nil{return false,nil,err}
 var envelope struct{StatusCode int `json:"status_code"`;StatusMessage string `json:"status_message"`;Tasks []struct{StatusCode int `json:"status_code"`;StatusMessage string `json:"status_message"`;Result []map[string]any `json:"result"`} `json:"tasks"`}
 if err=json.Unmarshal(raw,&envelope);err!=nil{return false,nil,fmt.Errorf("decode review task result: %w",err)}
 if envelope.StatusCode!=20000{return false,nil,fmt.Errorf("DataForSEO task_get failed: %s",envelope.StatusMessage)}
 if len(envelope.Tasks)==0{return false,nil,errors.New("DataForSEO review task was not found")}
 task:=envelope.Tasks[0]
 switch task.StatusCode{case 20100,40601,40602:return true,nil,nil}
 if task.StatusCode!=20000{
  if task.StatusCode==40501&&strings.Contains(strings.ToLower(task.StatusMessage),"no search results"){return false,nil,nil}
  return false,nil,fmt.Errorf("DataForSEO review task failed: %s",task.StatusMessage)
 }
 if len(task.Result)==0{return false,nil,nil};return false,task.Result[0],nil
}
