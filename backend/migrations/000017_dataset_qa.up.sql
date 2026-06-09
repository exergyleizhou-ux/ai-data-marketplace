-- 000017: dataset Q&A.  Buyer asks public question on a dataset; seller answers.
-- Both sides public so future buyers see prior answers (discovery value).
CREATE TABLE IF NOT EXISTS dataset_questions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id      UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    asker_id        UUID NOT NULL REFERENCES users(id),
    body            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open', 'answered', 'hidden')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dataset_answers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id     UUID NOT NULL REFERENCES dataset_questions(id) ON DELETE CASCADE,
    answerer_id     UUID NOT NULL REFERENCES users(id),
    body            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dataset_questions_dataset
    ON dataset_questions (dataset_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dataset_answers_question
    ON dataset_answers (question_id, created_at);
