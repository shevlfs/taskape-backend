package handlers

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthHandler struct {
	Pool *pgxpool.Pool
}

type UserHandler struct {
	Pool *pgxpool.Pool
}

type TaskHandler struct {
	Pool *pgxpool.Pool
}

type FriendHandler struct {
	Pool *pgxpool.Pool
}

type EventHandler struct {
	Pool *pgxpool.Pool
}

func NewEventHandler(pool *pgxpool.Pool) *EventHandler {
	return &EventHandler{Pool: pool}
}

func NewAuthHandler(pool *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{Pool: pool}
}

func NewUserHandler(pool *pgxpool.Pool) *UserHandler {
	return &UserHandler{Pool: pool}
}

func NewTaskHandler(pool *pgxpool.Pool) *TaskHandler {
	return &TaskHandler{Pool: pool}
}

func NewFriendHandler(pool *pgxpool.Pool) *FriendHandler {
	return &FriendHandler{Pool: pool}
}
