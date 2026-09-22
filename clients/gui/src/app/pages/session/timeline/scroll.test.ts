// Tests for the Wayshard timeline scroll helpers.
import { describe, expect, test } from "bun:test"
import { clampScrollTop, isAtBottom, nextWindow, windowRows } from "./scroll"

describe("isAtBottom", () => {
  test("true at the very end", () => {
    expect(isAtBottom({ scrollTop: 900, clientHeight: 100, scrollHeight: 1000 })).toBe(true)
  })
  test("true within the threshold", () => {
    expect(isAtBottom({ scrollTop: 870, clientHeight: 100, scrollHeight: 1000 })).toBe(true)
  })
  test("false when scrolled away", () => {
    expect(isAtBottom({ scrollTop: 100, clientHeight: 100, scrollHeight: 1000 })).toBe(false)
  })
})

describe("clampScrollTop", () => {
  test("clamps to the content height", () => {
    expect(clampScrollTop(5000, { scrollTop: 0, clientHeight: 100, scrollHeight: 1000 })).toBe(900)
  })
  test("never negative", () => {
    expect(clampScrollTop(-10, { scrollTop: 0, clientHeight: 100, scrollHeight: 1000 })).toBe(0)
  })
})

describe("windowRows", () => {
  const rows = Array.from({ length: 100 }, (_, i) => i)
  test("bounds to the newest rows", () => {
    expect(windowRows(rows, 60)).toEqual(rows.slice(40))
  })
  test("returns all rows when under the limit", () => {
    expect(windowRows(rows.slice(0, 10), 60)).toEqual(rows.slice(0, 10))
  })
  test("a non-positive limit is treated as unbounded", () => {
    expect(windowRows(rows, 0)).toEqual(rows)
  })
})

describe("nextWindow", () => {
  test("grows by the step, capped at total", () => {
    expect(nextWindow(60, 100, 60)).toBe(100)
    expect(nextWindow(60, 200, 60)).toBe(120)
  })
})
