-- migrations/001_init_schema.sql
-- ArkHub 数据库初始化脚本
-- 创建数据库、表、索引、初始用户

-- 创建数据库（如果由 docker-compose 自动创建则跳过）
-- CREATE DATABASE arhub;

\c arhub;

-- ============================================================
-- 1. 用户资产表
-- ============================================================
CREATE TABLE IF NOT EXISTS user_balances (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    asset VARCHAR(20) NOT NULL,          -- BTC, ETH, USDT 等
    available DECIMAL(36, 18) NOT NULL DEFAULT 0,
    frozen DECIMAL(36, 18) NOT NULL DEFAULT 0,
    total DECIMAL(36, 18) NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, asset)
);

CREATE INDEX IF NOT EXISTS idx_user_balances_user_id ON user_balances(user_id);

-- ============================================================
-- 2. 订单表
-- ============================================================
CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL UNIQUE,
    user_id VARCHAR(64) NOT NULL,
    symbol VARCHAR(20) NOT NULL,            -- BTC/USDT
    side VARCHAR(4) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    type VARCHAR(20) NOT NULL DEFAULT 'LIMIT', -- LIMIT, MARKET
    price DECIMAL(36, 18),
    quantity DECIMAL(36, 18) NOT NULL,
    filled_quantity DECIMAL(36, 18) NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING', -- PENDING, PARTIAL, FILLED, CANCELLED
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_orders_symbol ON orders(symbol);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at);

-- ============================================================
-- 3. 成交记录表
-- ============================================================
CREATE TABLE IF NOT EXISTS trades (
    id BIGSERIAL PRIMARY KEY,
    trade_id VARCHAR(64) NOT NULL UNIQUE,
    buy_order_id VARCHAR(64) NOT NULL,
    sell_order_id VARCHAR(64) NOT NULL,
    buy_user_id VARCHAR(64) NOT NULL,
    sell_user_id VARCHAR(64) NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    price DECIMAL(36, 18) NOT NULL,
    quantity DECIMAL(36, 18) NOT NULL,
    amount DECIMAL(36, 18) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades(symbol);
CREATE INDEX IF NOT EXISTS idx_trades_created_at ON trades(created_at);

-- ============================================================
-- 4. NFT 资产表
-- ============================================================
CREATE TABLE IF NOT EXISTS nft_assets (
    id BIGSERIAL PRIMARY KEY,
    token_id VARCHAR(128) NOT NULL,
    contract_address VARCHAR(42) NOT NULL,
    owner VARCHAR(42) NOT NULL,
    name VARCHAR(255),
    description TEXT,
    image_url TEXT,
    rarity_score DECIMAL(10, 4),
    heat_score DECIMAL(10, 4),
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(token_id, contract_address)
);

CREATE INDEX IF NOT EXISTS idx_nft_owner ON nft_assets(owner);
CREATE INDEX IF NOT EXISTS idx_nft_heat ON nft_assets(heat_score DESC);

-- ============================================================
-- 5. 回购记录表
-- ============================================================
CREATE TABLE IF NOT EXISTS buyback_records (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    asset VARCHAR(20) NOT NULL,
    amount DECIMAL(36, 18) NOT NULL,
    price DECIMAL(36, 18) NOT NULL,
    total_value DECIMAL(36, 18) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_buyback_asset ON buyback_records(asset);
CREATE INDEX IF NOT EXISTS idx_buyback_created_at ON buyback_records(created_at);

-- ============================================================
-- 6. 链上同步断点表
-- ============================================================
CREATE TABLE IF NOT EXISTS sync_checkpoint (
    id BIGSERIAL PRIMARY KEY,
    chain VARCHAR(20) NOT NULL,
    contract_address VARCHAR(42),
    block_number BIGINT NOT NULL DEFAULT 0,
    block_hash VARCHAR(66),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(chain, contract_address)
);

-- ============================================================
-- 7. 风控事件表
-- ============================================================
CREATE TABLE IF NOT EXISTS risk_events (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(64) NOT NULL UNIQUE,
    rule_name VARCHAR(255) NOT NULL,
    risk_level VARCHAR(20) NOT NULL,        -- LOW, MEDIUM, HIGH, CRITICAL
    user_id VARCHAR(64),
    transaction_id VARCHAR(64),
    details JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_risk_user_id ON risk_events(user_id);
CREATE INDEX IF NOT EXISTS idx_risk_level ON risk_events(risk_level);
CREATE INDEX IF NOT EXISTS idx_risk_created_at ON risk_events(created_at);

-- ============================================================
-- 初始化示例数据
-- ============================================================

-- 示例用户资产
INSERT INTO user_balances (user_id, asset, available, frozen, total)
VALUES
    ('user_001', 'USDT', 10000.00, 0, 10000.00),
    ('user_001', 'BTC', 0.5, 0, 0.5),
    ('user_002', 'USDT', 5000.00, 0, 5000.00),
    ('user_002', 'ETH', 2.0, 0, 2.0)
ON CONFLICT (user_id, asset) DO NOTHING;
