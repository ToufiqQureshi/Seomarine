package keywords

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"

 "github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const businessQuestionsPath="/v3/business_data/google/questions_and_answers/live"
type BusinessQuestionsInput struct { Keyword,Coordinate,LanguageCode string; Depth int }
type BusinessQuestionsProvider interface { BusinessQuestions(context.Context,string,BusinessQuestionsInput)([]map[string]any,error) }

// BusinessQuestions fetches Google Business Profile Q&A. Empty provider results
// are a valid paid response for businesses without questions.
func (p DataForSEOProvider) BusinessQuestions(ctx context.Context,org string,in BusinessQuestionsInput)([]map[string]any,error){
 results,err:=p.post(ctx,org,businessQuestionsPath,map[string]any{"keyword":in.Keyword,"location_coordinate":in.Coordinate,"language_code":in.LanguageCode,"depth":in.Depth})
 if taskErr,ok:=errors.AsType[*dataforseo.TaskError](err);ok&&taskErr.StatusCode==40501{return []map[string]any{},nil}
 if err!=nil{return nil,err}
 out:=make([]map[string]any,0)
 for _,raw:=range results{var entry struct{Items []map[string]any `json:"items"`;WithoutAnswers []map[string]any `json:"items_without_answers"`};if e:=json.Unmarshal(raw,&entry);e!=nil{return nil,fmt.Errorf("decode business questions: %w",e)};out=append(out,entry.Items...);out=append(out,entry.WithoutAnswers...)}
 return out,nil
}
