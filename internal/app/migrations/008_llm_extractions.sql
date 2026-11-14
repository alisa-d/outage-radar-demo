-- Raw LLM output per unique Notice text: the extractor runs once per text, ever.
CREATE TABLE llm_extractions (
    text_hash text PRIMARY KEY,
    result    jsonb NOT NULL
);
