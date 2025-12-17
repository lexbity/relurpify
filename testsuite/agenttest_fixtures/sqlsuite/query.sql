-- Return each user name and post count, descending by count then name.
SELECT u.name, COUNT(p.id) AS post_count
FROM users u
LEFT JOIN posts p ON p.user_id = u.id
GROUP BY u.id
ORDER BY post_count DESC, u.name ASC;

