package main

import (
	"container/list"
	"github.com/googollee/go-socket.io"
	"log"
	"net/http"
	"time"
)

type Event struct {
	EvtType string
	User string
	Timestamp int
	Text string
}

type Subscription struct {
	Archive []Event
	New <-chan Event
}

// 이벤트 생성 함수
func NewEvent(evtType, user, msg string) Event {
	return Event{evtType, user, int(time.Now().Unix()), msg}
}

var (
	subscribe = make(chan (chan<- Subscription), 10)
	unsubscribe = make(chan (<-chan Event), 10)
	publish = make(chan Event, 10)
)

// 새로운 사용자가 들어왔을때 이벤트를 구독
func Subscribe() Subscription {
	c := make(chan Subscription)
	subscribe <-c
	return <-c // subscribe 구조체가 올 때까지 대기한 뒤 꺼내서 리턴
}

// 사용자가 나갔을때 구독 취소
func (s Subscription) Cancel() {
	unsubscribe <- s.New

	for {
		select {
		case _, ok := <- s.New:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// 사용자가 들어왔을 때 이벤트 발행
func Join(user string) {
	publish <- NewEvent("join", user, "")
}

// 사용자가 채팅 메시지를 보냈을 때 이벤트 발행
func Say(user, message string) {
	publish <- NewEvent("message", user, message)
}

// 사용자가 나갔을 때 이벤트 발생
func Leave(user string)  {
	publish <- NewEvent("leave", user, "")
}

func Chatroom() {
	archive := list.New()
	subscribers := list.New()

	for {
		select {
		case c := <-subscribe: // 새로운 사용자
			var events []Event
			for e := archive.Front(); e != nil; e = e.Next() { // 이벤트가 있으면 events 에 저장
				events = append(events, e.Value.(Event))
			}
			subscriber := make(chan Event, 10)
			subscribers.PushBack(subscriber)

			c <- Subscription{events, subscriber}

		case event := <-publish:
			for e := subscribers.Front(); e !=nil; e = e.Next() {
				subscriber := e.Value.(chan Event) // 이벤트가 들어오면 모든 사용자에게 전달
				subscriber <- event
			}
			if archive.Len() >= 20 {
				archive.Remove(archive.Front())
			}
			archive.PushBack(event) // 현재 이벤트 저장

		case c := <-unsubscribe: // 사용자가 나감
			for e := subscribers.Front(); e !=nil; e = e.Next() {
				subscriber := e.Value.(chan Event)
				if subscriber == c {
					// 동일 사용자면 목록에서 삭제
					subscribers.Remove(e)
					break
				}
			}
		}
	}
}

func main() {
	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}

	go Chatroom()

	server.On("connection", func(so socketio.Socket) {
		s := Subscribe()
		Join(so.Id())

		for _, event := range s.Archive { // 지금껏 쌓인 이벤트를 접속자에게 보냄
			so.Emit("event", event)
		}

		newMessages := make(chan string)

		// 웹브라우저에서 보내오는 채팅 메시지를 받는 콜백
		so.On("message", func(msg string) {
			newMessages <- msg
		})

		// 웹브라우저 접속이 끊김
		so.On("disconnection", func() {
			Leave(so.Id())
			s.Cancel()
		})

		go func() {
			for {
				select {
				case event := <-s.New:
					so.Emit("event", event)
				case msg := <-newMessages:
					Say(so.Id(), msg)
				}
			}
		}()
	})

	http.Handle("/socket.io/", server)

	http.Handle("/", http.FileServer(http.Dir(".")))

	log.Fatal(http.ListenAndServe(":80", nil))
}