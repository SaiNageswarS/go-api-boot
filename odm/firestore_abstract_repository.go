package odm

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

type FirestoreBootRepository[T any] interface {
	Save(model DbModel) chan error
	FindOneById(id string) (chan *T, chan error)
	IsExistsById(docID string) bool
	CountDocuments(query firestore.Query) (chan int64, chan error)
	FindOne(filters bson.M) (chan *T, chan error)
	Find(filters bson.M, sort bson.D, limit, skip int) (chan []T, chan error)
	DeleteById(id string) chan error
	DeleteOne(filters bson.M) chan error
}

type UnimplementedFirestoreBootRepository[T any] struct {
	Database       string
	CollectionName string
}

func (r *UnimplementedFirestoreBootRepository[T]) db() *firestore.Client {
	return GetFireStoreClient(r.Database)
}

func (r *UnimplementedFirestoreBootRepository[T]) Save(model DbModel) chan error {
	ch := make(chan error)
	ctx := context.Background()
	go func() {
		id := model.Id()
		collection := r.db().Collection(r.CollectionName)
		documentRef := collection.Doc(id)
		document := make(map[string]interface{})
		if r.IsExistsById(id) {
			document["updatedOn"] = time.Now().Unix()
		} else {
			document["createdOn"] = time.Now().Unix()
		}
		documentRef.Set(ctx, model)
		documentRef.Set(ctx, document, firestore.MergeAll)
		ch <- nil
	}()

	return ch
}

func (r *UnimplementedFirestoreBootRepository[T]) IsExistsById(docID string) bool {
	resCh := make(chan bool)
	go func() {
		docRef := r.db().Collection(r.CollectionName).Doc(docID)
		docSnap, err := docRef.Get(context.Background())
		if err != nil {
			fmt.Println("hit id existanc eerro")
			logger.Error("error checking document existence", zap.Error(err))
			resCh <- false
			return
		}
		if docSnap.Exists() {
			resCh <- true
			return
		}
	}()
	fmt.Println("request processed")
	res := <-resCh
	return res
}

func (r *UnimplementedFirestoreBootRepository[T]) CountDocuments(query firestore.Query) (chan int64, chan error) {
	countChan := make(chan int64)
	errChan := make(chan error)

	go func() {
		ctx := context.Background()
		documents, err := query.Documents(ctx).GetAll()

		if err != nil {
			errChan <- err
			return
		}

		countChan <- int64(len(documents))
	}()

	return countChan, errChan
}

func (r *UnimplementedFirestoreBootRepository[T]) Find(filters bson.M, sort bson.D, limit, skip int) (chan []T, chan error) {
	resultChan := make(chan []T)
	errorChan := make(chan error)

	go func() {
		ctx := context.Background()
		q := r.db().Collection(r.CollectionName).Query

		for key, value := range filters {
			if key == "_id" {
				reschan, errchan := r.FindOneById(value.(string))
				select {
				case res := <-reschan:
					if res != nil {
						resultChan <- []T{*res}
					} else {
						resultChan <- nil
					}
				case err := <-errchan:
					errorChan <- err
				}
				return
			} else {
				q = q.Where(key, "==", value)
			}
		}

		for _, s := range sort {
			q = q.OrderBy(s.Value.(string), firestore.Asc)
		}

		if limit != 0 {
			q = q.Limit(limit)
		}

		if skip != 0 {
			q = q.Offset(skip)
		}

		iter := q.Documents(ctx)
		defer iter.Stop()

		var result []T
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				errorChan <- err
				return
			}

			var item T
			if err := doc.DataTo(&item); err != nil {
				errorChan <- err
				return
			}
			result = append(result, item)
		}

		resultChan <- result
	}()

	return resultChan, errorChan
}

func (r *UnimplementedFirestoreBootRepository[T]) FindOne(filters bson.M) (chan *T, chan error) {
	singleResultChan := make(chan *T)
	errorChan := make(chan error)
	resultChan, errChan := r.Find(filters, nil, 1, 0)
	go func() {
		select {
		case result := <-resultChan:
			if len(result) > 0 {
				singleResultChan <- &result[0]
			} else {
				singleResultChan <- nil
			}
		case err := <-errChan:
			errorChan <- err
		}
	}()
	return singleResultChan, errorChan
}

func (r *UnimplementedFirestoreBootRepository[T]) FindOneById(id string) (chan *T, chan error) {
	resultChan := make(chan *T)
	errorChan := make(chan error)
	ctx := context.Background()
	go func() {
		docRef := r.db().Collection(r.CollectionName).Doc(id)
		fmt.Println(docRef)
		docSnap, err := docRef.Get(ctx)
		if err != nil {
			fmt.Println("hit1")
			errorChan <- err
			return
		}
		if !docSnap.Exists() {
			fmt.Println("hit2")
			errorChan <- fmt.Errorf("document with ID %s not found", id)
			return
		}
		var result T
		if err := docSnap.DataTo(&result); err != nil {
			fmt.Println("hit3")
			errorChan <- fmt.Errorf("error decoding document with ID %s: %w", id, err)
			return
		}
		fmt.Println("hit4")
		resultChan <- &result
	}()
	return resultChan, errorChan
}

func (r *UnimplementedFirestoreBootRepository[T]) DeleteById(id string) chan error {
	ch := make(chan error)
	ctx := context.Background()
	go func() {
		collection := r.db().Collection(r.CollectionName)
		_, err := collection.Doc(id).Delete(ctx)
		if err != nil {
			ch <- err
			return
		}
		ch <- nil
	}()
	return ch
}

func (r *UnimplementedFirestoreBootRepository[T]) DeleteOne(filters bson.M) chan error {
	ch := make(chan error)
	ctx := context.Background()
	go func() {
		q := r.db().Collection(r.CollectionName).Query
		for key, value := range filters {
			q = q.Where(key, "==", value)
		}
		it := q.Documents(ctx)
		doc, err := it.Next()
		if err == iterator.Done {
			ch <- fmt.Errorf("no documents found matching filters")
			return
		} else if err != nil {
			ch <- fmt.Errorf("failed to retrieve document: %w", err)
		}

		// Delete the document
		_, err = doc.Ref.Delete(ctx)
		if err != nil {
			ch <- fmt.Errorf("failed to delete document: %w", err)
			return
		}
	}()
	return ch
}
