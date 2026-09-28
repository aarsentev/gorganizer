-- Telegram message id of the reminder, needed to delete it later (reminders.cleanup).
ALTER TABLE sent ADD COLUMN message_id INTEGER;
