// @ts-check
import http from "k6/http";
import { check } from "k6";

// This script establishes your Day 0 baseline.
// When you interview at GreenNode, you will use this baseline to prove
// the minimal overhead your custom load-balancing and rate-limiting logic introduces.
export const options = {
  vus: 100, // 100 Concurrent Virtual Users
  duration: "10s",
};

export default function () {
  const url = "http://localhost:8080/v1/chat/completions";
  const payload = JSON.stringify({
    model: "mock-llama-4",
    messages: [
      { role: "user", content: "Explain distributed system consensus." },
    ],
    stream: true,
  });

  const params = {
    headers: {
      "Content-Type": "application/json",
    },
  };

  const res = http.post(url, payload, params);

  check(res, {
    "is status 200": (r) => r.status === 200,
  });
}
