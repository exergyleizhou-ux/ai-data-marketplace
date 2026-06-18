-- A dataset question has at most one answer (the model + LEFT JOIN assume 1:1),
-- but dataset_answers had no unique key on question_id. AnswerQuestion's
-- read-then-write guard (q.Answer == nil) races, so a double-submit could insert
-- two answers. Make question_id unique so the second insert is rejected.

-- Collapse any pre-existing duplicates, keeping the most recent answer.
DELETE FROM dataset_answers a
USING (
    SELECT id,
           row_number() OVER (PARTITION BY question_id ORDER BY created_at DESC, id DESC) AS rn
    FROM dataset_answers
) d
WHERE a.id = d.id AND d.rn > 1;

ALTER TABLE dataset_answers
    ADD CONSTRAINT dataset_answers_question_id_key UNIQUE (question_id);
