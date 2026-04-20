local paths = {
  "/v1/users/me",
  "/v1/venues/list-venues?sport=futsal&limit=6&page=1",
  "/v1/store/cart",
  "/v1/store/orders?page=1&limit=10",
  "/v1/venues/5/available-times?date=2026-02-16T13:45:00%2B05:45"
}

local i = 1

request = function()
  local path = paths[i]
  i = i + 1
  if i > #paths then i = 1 end

  return wrk.format(
    "GET",
    path,
    {
      ["Authorization"] = "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJLaGVsIiwiZXhwIjoxNzcxMjczOTMyLCJpYXQiOjE3NzEwMTQ3MzIsImlzcyI6IktoZWwiLCJuYmYiOjE3NzEwMTQ3MzIsInJvbGUiOiJ2ZW51ZV9vd25lciIsInN1YiI6NTV9.eYvqISADyJIwnlxN5LaSQAY0sx14b68ZnJJW9Di6BUc",
      ["Content-Type"] = "application/json",
    }
  )
end
