#!/bin/sh
# Smoke test against the scale1m API on :4600. Usage: smoke.sh [n1 n2 n3]  (learner numbers)
# Default: one random sample learner (n % 100 = 0, has attempts) and two random learners.
API=http://localhost:4600/api/v1
CUR=0153f4d4-9f3d-4abe-8de0-34a72486ae55
r() { python3 -c "import random; print(random.randint(1, 1000000))"; }
LEARNERS=${*:-"$(( $(r) / 100 * 100 + 100 )) $(r) $(r)"}

call() { # method path [json] -> prints "status time_ms path"; body in /tmp-free $BODY
  m=$1 p=$2 d=$3
  if [ -n "$d" ]; then out=$(curl -s -o "$BODY" -w "%{http_code} %{time_total}" -X "$m" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' --data "$d" "$API$p")
  else out=$(curl -s -o "$BODY" -w "%{http_code} %{time_total}" -X "$m" -H "Authorization: Bearer $TOKEN" "$API$p"); fi
  set -- $out
  printf "%s %6.1fms  %-6s %-38s %s\n" "$1" "$(echo "$2*1000" | bc)" "$m" "$p" "$(jq -c "$JQ" "$BODY" 2>/dev/null | cut -c1-150)"
}

BODY=$(mktemp "${TMPDIR:-.}/smoke.XXXXXX")
for n in $LEARNERS; do
  TOKEN=scale1m-learner-$(printf %07d $n)
  echo "== learner $n  (sample=$([ $((n % 100)) -eq 0 ] && echo yes || echo no))"
  for pass in cold warm; do
    echo "-- $pass"
    JQ='.data.goal | {title, examDate, daysRemaining}'; call GET /goals
    JQ='.data | {attempts, weakest: (.weakestTopics | length)}'; call GET /learner/summary
    JQ='.data | {action: .recommendation.action, topic: .recommendation.topic.name, difficulty: .recommendation.difficulty, questions: (.questions | length)}'; call GET "/recommendations/next?minutes=20"
    QID=$(jq -r '.data.questions[0].id // empty' "$BODY")
    JQ='.data | {due: (.due | length), today}'; call GET /revision/due
    JQ='.data.readiness | {score, band, confidence}'; call GET /readiness
    JQ='.data.subjects | map([.topicsStarted, .averageMastery])'; call GET /learner/subjects
    JQ='.data | {activeDays: ([.calendar.days[] | select(.count > 0)] | length), badges: (.badges | length)}'; call GET /learner/activity
    JQ='.data | {subjects: (.subjects | length), topics: ([.subjects[].topics[]] | length)}'; call GET "/sources/map/$CUR"
    if [ -n "$QID" ]; then
      JQ='.data | {isCorrect, mastery: {before: .mastery.before, after: .mastery.after, n: .mastery.attemptsOnTopic}}'; call POST /attempts "{\"questionId\":\"$QID\",\"selectedOptions\":[0],\"timeTakenSeconds\":31}"
    fi
  done
done
echo "== AI-backed endpoints with AI off (learner ${LEARNERS%% *})"
TOKEN=scale1m-learner-$(printf %07d ${LEARNERS%% *})
JQ='.error'; call POST /curricula/resolve '{"goalText":"Class 12 Chemistry"}'
JQ='.error'; call POST /questions/generate "{\"topicId\":\"f282b92a-7caa-40c0-8f39-1e3f53dd1937\",\"count\":3}"
JQ='.error // (.data | keys)'; call POST /ai/tutor '{"question":"Why does a spoon look bent in water?"}'
echo "== bad token"
TOKEN=scale1m-learner-0000000; JQ='.error'; call GET /goals
rm -f "$BODY"
