package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/keywords"
)

var businessQuestionFields=[]string{"rank_absolute","question_id","question_text","original_question_text","profile_name","time_ago","timestamp"}
var businessAnswerFields=[]string{"answer_id","answer_text","original_answer_text","profile_name","time_ago","timestamp"}
func getGoogleBusinessQuestionsTool()*tool{return &tool{Name:"get_google_business_questions",Title:"Get Google business questions",Description:"Fetch Google Business Profile questions and answers for one business by name, CID, or place ID near a coordinate. Charges credits.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"businessName":{"type":"string","minLength":1,"maxLength":200},"cid":{"type":"string","minLength":1,"maxLength":64},"placeId":{"type":"string","minLength":1,"maxLength":256},"near":{"type":"object","properties":{"latitude":{"type":"number","minimum":-90,"maximum":90},"longitude":{"type":"number","minimum":-180,"maximum":180},"radiusKm":{"type":"number","minimum":1,"maximum":100000}},"required":["latitude","longitude"],"additionalProperties":false},"depth":{"type":"integer","minimum":1,"maximum":100},"languageCode":{"type":"string","minLength":2,"maxLength":8}},"required":["projectId","near"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"questions":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`),
 Annotations:map[string]any{"readOnlyHint":false,"openWorldHint":true,"destructiveHint":false},Handler:handleGetGoogleBusinessQuestions}}
func handleGetGoogleBusinessQuestions(ctx context.Context,raw json.RawMessage,env *callEnv)(*callResult,error){
 var a struct{ProjectID string `json:"projectId"`;BusinessName *string `json:"businessName"`;CID *string `json:"cid"`;PlaceID *string `json:"placeId"`;Near struct{Latitude *float64 `json:"latitude"`;Longitude *float64 `json:"longitude"`;RadiusKm *float64 `json:"radiusKm"`} `json:"near"`;Depth int `json:"depth"`;LanguageCode string `json:"languageCode"`}
 if e:=json.Unmarshal(raw,&a);e!=nil||strings.TrimSpace(a.ProjectID)==""{return nil,newAppErrorf("VALIDATION_ERROR","projectId is required")}
 count:=0;for _,v:=range []*string{a.BusinessName,a.CID,a.PlaceID}{if v!=nil{count++}};if count!=1{return nil,newAppErrorf("VALIDATION_ERROR","Provide exactly one business identifier: businessName, cid, or placeId.")}
 identifier:="";switch{case a.BusinessName!=nil&&strings.TrimSpace(*a.BusinessName)!="":if len(*a.BusinessName)>200{return nil,newAppErrorf("VALIDATION_ERROR","businessName is too long")};identifier=strings.TrimSpace(*a.BusinessName);case a.CID!=nil&&strings.TrimSpace(*a.CID)!="":if len(*a.CID)>64{return nil,newAppErrorf("VALIDATION_ERROR","cid is too long")};identifier="cid:"+strings.TrimSpace(*a.CID);case a.PlaceID!=nil&&strings.TrimSpace(*a.PlaceID)!="":if len(*a.PlaceID)>256{return nil,newAppErrorf("VALIDATION_ERROR","placeId is too long")};identifier="place_id:"+strings.TrimSpace(*a.PlaceID);default:return nil,newAppErrorf("VALIDATION_ERROR","Business identifier must not be empty.")}
 if a.Near.Latitude==nil||a.Near.Longitude==nil||*a.Near.Latitude < -90||*a.Near.Latitude>90||*a.Near.Longitude < -180||*a.Near.Longitude>180{return nil,newAppErrorf("VALIDATION_ERROR","near coordinates are required and must be valid")}
 radius:=10000.0;if a.Near.RadiusKm!=nil{radius=*a.Near.RadiusKm};if radius<1||radius>100000{return nil,newAppErrorf("VALIDATION_ERROR","radiusKm must be from 1 to 100000")}
 depth:=a.Depth;if depth==0{depth=20};if depth<1||depth>100{return nil,newAppErrorf("VALIDATION_ERROR","depth must be from 1 to 100")}
 if a.LanguageCode!=""&&(len(strings.TrimSpace(a.LanguageCode))<2||len(strings.TrimSpace(a.LanguageCode))>8){return nil,newAppErrorf("VALIDATION_ERROR","languageCode must be 2 to 8 characters")}
 access,e:=env.h.authorizeProject(ctx,env.auth,a.ProjectID);if e!=nil{return nil,e};if env.deps.KeywordResearch==nil{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Keyword research provider is not configured.")}
 provider,ok:=env.deps.KeywordResearch.Data.(keywords.BusinessQuestionsProvider);if !ok{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Google Business questions are not configured.")}
 lang:=strings.TrimSpace(a.LanguageCode);if lang==""{lang=access.Project.LanguageCode}
 coord:=fmt.Sprintf("%s,%s,%d",coordinatePart(*a.Near.Latitude),coordinatePart(*a.Near.Longitude),int(radius*1000))
 rawRows,e:=provider.BusinessQuestions(ctx,access.Auth.OrganizationID,keywords.BusinessQuestionsInput{Keyword:identifier,Coordinate:coord,LanguageCode:lang,Depth:depth});if e!=nil{env.deps.Logger.ErrorContext(ctx,"MCP Google Business questions","project_id",access.Project.ID,"err",e);return nil,newAppErrorf("INTERNAL_ERROR","Unable to fetch Google Business questions.")}
 questions:=make([]map[string]any,0,len(rawRows));for _,row:=range rawRows{item:=pickMCPFields(row,businessQuestionFields);answers,_:=row["items"].([]any);if answers==nil{item["items"]=nil}else{trimmed:=make([]map[string]any,0,len(answers));for _,a:=range answers{if m,ok:=a.(map[string]any);ok{trimmed=append(trimmed,pickMCPFields(m,businessAnswerFields))}};item["items"]=trimmed};questions=append(questions,item)}
 lines:=[]string{fmt.Sprintf("Fetched %d Google Business Q&A rows for %s.",len(questions),identifier)};for _,q:=range questions{lines=append(lines,fmt.Sprintf("%v | %v | %v | %v",q["question_text"],q["profile_name"],q["time_ago"],q["items"]))}
 return mcpResponse(strings.Join(lines,"\n"),map[string]any{"questions":questions},metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID,nil)}),nil
}
func coordinatePart(v float64)string{return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.5f",v),"0"),".")}
func pickMCPFields(row map[string]any,fields []string)map[string]any{out:=map[string]any{};for _,field:=range fields{if v,ok:=row[field];ok{out[field]=v}};return out}
