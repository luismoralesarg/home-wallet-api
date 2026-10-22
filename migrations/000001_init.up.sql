CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username      text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE categories (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('income', 'expense')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (name, kind)
);

CREATE TABLE exchange_rates (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    date          date NOT NULL UNIQUE,
    currency      text NOT NULL DEFAULT 'USD' CHECK (currency = 'USD'),
    official_buy  numeric(14,2) NOT NULL,
    official_sell numeric(14,2) NOT NULL,
    fetched_at    timestamptz NOT NULL DEFAULT now()
);

-- Ingresos y gastos pagados con efectivo/cuenta.
CREATE TABLE transactions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind        text NOT NULL CHECK (kind IN ('income', 'expense')),
    amount      numeric(14,2) NOT NULL CHECK (amount > 0),
    currency    text NOT NULL CHECK (currency IN ('ARS', 'USD')),
    fx_rate     numeric(14,2) NOT NULL DEFAULT 1,
    amount_ars  numeric(14,2) NOT NULL,
    category_id uuid NOT NULL REFERENCES categories(id),
    description text NOT NULL DEFAULT '',
    created_by  uuid NOT NULL REFERENCES users(id),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX transactions_occurred_at_idx ON transactions (occurred_at);
CREATE INDEX transactions_category_idx ON transactions (category_id);

CREATE TABLE cards (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    last4       text NOT NULL DEFAULT '' CHECK (last4 = '' OR last4 ~ '^\d{4}$'),
    closing_day smallint NOT NULL CHECK (closing_day BETWEEN 1 AND 31),
    due_day     smallint NOT NULL CHECK (due_day BETWEEN 1 AND 31),
    created_by  uuid NOT NULL REFERENCES users(id),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE card_purchases (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    card_id      uuid NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    amount       numeric(14,2) NOT NULL CHECK (amount > 0),
    currency     text NOT NULL CHECK (currency IN ('ARS', 'USD')),
    fx_rate      numeric(14,2) NOT NULL DEFAULT 1,
    amount_ars   numeric(14,2) NOT NULL,
    installments smallint NOT NULL DEFAULT 1 CHECK (installments >= 1),
    category_id  uuid NOT NULL REFERENCES categories(id),
    description  text NOT NULL DEFAULT '',
    created_by   uuid NOT NULL REFERENCES users(id),
    purchased_at timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX card_purchases_card_idx ON card_purchases (card_id);
CREATE INDEX card_purchases_purchased_at_idx ON card_purchases (purchased_at);

-- Una fila por cuota, asignada al resumen según el cierre de la tarjeta.
-- statement_month: primer día del mes de cierre del resumen.
CREATE TABLE card_purchase_installments (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_id        uuid NOT NULL REFERENCES card_purchases(id) ON DELETE CASCADE,
    installment_number smallint NOT NULL CHECK (installment_number >= 1),
    statement_month    date NOT NULL CHECK (date_trunc('month', statement_month) = statement_month),
    amount_ars         numeric(14,2) NOT NULL,
    UNIQUE (purchase_id, installment_number)
);
CREATE INDEX card_installments_month_idx ON card_purchase_installments (statement_month);

-- El total no se guarda: cuotas + previous_balance_adjustment_ars.
CREATE TABLE card_statements (
    id                             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    card_id                        uuid NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    month                          date NOT NULL CHECK (date_trunc('month', month) = month),
    due_date                       date NOT NULL,
    previous_balance_adjustment_ars numeric(14,2) NOT NULL DEFAULT 0,
    paid                           boolean NOT NULL DEFAULT false,
    paid_at                        timestamptz,
    UNIQUE (card_id, month)
);

CREATE TABLE loans (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    kind        text NOT NULL CHECK (kind IN ('loan', 'vehicle')),
    currency    text NOT NULL CHECK (currency IN ('ARS', 'USD')),
    base_amount numeric(14,2) NOT NULL CHECK (base_amount > 0),
    start_month date NOT NULL CHECK (date_trunc('month', start_month) = start_month),
    end_month   date NOT NULL CHECK (date_trunc('month', end_month) = end_month),
    created_by  uuid NOT NULL REFERENCES users(id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (end_month >= start_month)
);

CREATE TABLE loan_installments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    loan_id    uuid NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
    month      date NOT NULL CHECK (date_trunc('month', month) = month),
    amount     numeric(14,2) NOT NULL,
    amount_ars numeric(14,2) NOT NULL,
    paid       boolean NOT NULL DEFAULT false,
    UNIQUE (loan_id, month)
);

CREATE TABLE subscriptions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    amount     numeric(14,2) NOT NULL CHECK (amount > 0),
    currency   text NOT NULL CHECK (currency IN ('ARS', 'USD')),
    charge_day smallint NOT NULL CHECK (charge_day BETWEEN 1 AND 31),
    active     boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Usuarios: no hay registro, se crean con este procedure.
CREATE PROCEDURE sp_create_user(p_username text, p_password text)
LANGUAGE sql AS $$
    INSERT INTO users (username, password_hash)
    VALUES (p_username, crypt(p_password, gen_salt('bf')));
$$;

-- Devuelve el id del usuario si las credenciales son válidas, sino NULL.
CREATE FUNCTION fn_login(p_username text, p_password text)
RETURNS uuid
LANGUAGE sql STABLE AS $$
    SELECT id FROM users
    WHERE username = p_username
      AND password_hash = crypt(p_password, password_hash);
$$;

-- Categorías iniciales.
INSERT INTO categories (name, kind) VALUES
    ('Alquiler', 'expense'),
    ('Comida', 'expense'),
    ('Transporte', 'expense'),
    ('Salud', 'expense'),
    ('Extras', 'expense'),
    ('Sueldo', 'income'),
    ('Otros ingresos', 'income');
