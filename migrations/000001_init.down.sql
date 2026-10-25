DROP FUNCTION IF EXISTS fn_login(text, text);
DROP PROCEDURE IF EXISTS sp_create_user(text, text);
DROP TABLE IF EXISTS subscriptions, loan_installments, loans, card_statements,
    card_purchase_installments, card_purchases, cards, transactions,
    exchange_rates, categories, users;
