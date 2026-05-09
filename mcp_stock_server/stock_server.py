#!/usr/bin/env python3
import json
import sys
from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import Tool, TextContent
import akshare as ak

server = Server("stock-info")

@server.list_tools()
async def list_tools() -> list[Tool]:
    return [
        Tool(
            name="get_stock_quote",
            description="获取A股股票的实时行情数据，包括最新价、涨跌幅、成交量等关键信息",
            inputSchema={
                "type": "object",
                "properties": {
                    "stock_code": {
                        "type": "string",
                        "description": "股票代码，如 '000001'（平安银行）、'600519'（贵州茅台）、'000858'（五粮液）"
                    }
                },
                "required": ["stock_code"]
            }
        ),
        Tool(
            name="get_stock_info",
            description="获取股票的基本信息，包括公司名称、所属行业、市值等",
            inputSchema={
                "type": "object",
                "properties": {
                    "stock_code": {
                        "type": "string",
                        "description": "股票代码，如 '000001'、'600519'"
                    }
                },
                "required": ["stock_code"]
            }
        ),
        Tool(
            name="get_stock_realtime_quotes",
            description="批量获取多只股票的实时行情（支持沪深A股、ETF等）",
            inputSchema={
                "type": "object",
                "properties": {
                    "stock_codes": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "股票代码列表，如 ['000001', '600519', '000858']"
                    }
                },
                "required": ["stock_codes"]
            }
        ),
        Tool(
            name="get_index_realtime",
            description="获取大盘指数的实时行情，如上证指数、深证成指、创业板指等",
            inputSchema={
                "type": "object",
                "properties": {
                    "index_code": {
                        "type": "string",
                        "description": "指数代码，如 '000001'（上证指数）、'399001'（深证成指）、'399006'（创业板指）"
                    }
                },
                "required": ["index_code"]
            }
        ),
        Tool(
            name="get_stock_history",
            description="获取股票的历史K线数据",
            inputSchema={
                "type": "object",
                "properties": {
                    "stock_code": {
                        "type": "string",
                        "description": "股票代码，如 '000001'"
                    },
                    "period": {
                        "type": "string",
                        "description": "K线周期，可选值：daily/weekly/monthly（默认 daily）",
                        "enum": ["daily", "weekly", "monthly"],
                        "default": "daily"
                    },
                    "start_date": {
                        "type": "string",
                        "description": "开始日期，格式 YYYYMMDD，如 '20240101'"
                    },
                    "end_date": {
                        "type": "string",
                        "description": "结束日期，格式 YYYYMMDD，如 '20241231'"
                    }
                },
                "required": ["stock_code"]
            }
        ),
        Tool(
            name="search_stock",
            description="搜索股票，通过关键词搜索股票信息",
            inputSchema={
                "type": "object",
                "properties": {
                    "keyword": {
                        "type": "string",
                        "description": "搜索关键词，可以是股票代码或股票名称"
                    }
                },
                "required": ["keyword"]
            }
        )
    ]

@server.call_tool()
async def call_tool(name: str, arguments: dict) -> list[TextContent]:
    try:
        if name == "get_stock_quote":
            return await get_stock_quote(arguments.get("stock_code"))
        elif name == "get_stock_info":
            return await get_stock_info(arguments.get("stock_code"))
        elif name == "get_stock_realtime_quotes":
            return await get_stock_realtime_quotes(arguments.get("stock_codes", []))
        elif name == "get_index_realtime":
            return await get_index_realtime(arguments.get("index_code"))
        elif name == "get_stock_history":
            return await get_stock_history(
                arguments.get("stock_code"),
                arguments.get("period", "daily"),
                arguments.get("start_date"),
                arguments.get("end_date")
            )
        elif name == "search_stock":
            return await search_stock(arguments.get("keyword"))
        else:
            return [TextContent(type="text", text=f"Unknown tool: {name}")]
    except Exception as e:
        return [TextContent(type="text", text=f"Error: {str(e)}")]

async def get_stock_quote(stock_code: str) -> list[TextContent]:
    try:
        df = ak.stock_zh_a_spot_em()
        stock_data = df[df['代码'] == stock_code]

        if stock_data.empty:
            return [TextContent(type="text", text=f"未找到股票 {stock_code}，请检查代码是否正确")]

        row = stock_data.iloc[0]
        result = {
            "股票代码": row['代码'],
            "股票名称": row['名称'],
            "最新价": row['最新价'],
            "涨跌幅": f"{row['涨跌幅']}%",
            "涨跌额": row['涨跌额'],
            "成交量": row['成交量'],
            "成交额": row['成交额'],
            "振幅": f"{row['振幅']}%",
            "最高": row['最高'],
            "最低": row['最低'],
            "今开": row['今开'],
            "昨收": row['昨收'],
            "量比": row['量比'],
            "换手率": f"{row['换手率']}%",
            "市盈率-动态": row['市盈率-动态'],
            "市净率": row['市净率'],
            "总市值": row['总市值'],
            "流通市值": row['流通市值']
        }

        return [TextContent(type="text", text=json.dumps(result, ensure_ascii=False, indent=2))]
    except Exception as e:
        return [TextContent(type="text", text=f"获取股票行情失败: {str(e)}")]

async def get_stock_info(stock_code: str) -> list[TextContent]:
    try:
        df = ak.stock_individual_info_em(symbol=stock_code)
        result = {row['item']: row['value'] for _, row in df.iterrows()}
        return [TextContent(type="text", text=json.dumps(result, ensure_ascii=False, indent=2))]
    except Exception as e:
        return [TextContent(type="text", text=f"获取股票信息失败: {str(e)}")]

async def get_stock_realtime_quotes(stock_codes: list) -> list[TextContent]:
    try:
        df = ak.stock_zh_a_spot_em()
        stock_data = df[df['代码'].isin(stock_codes)]

        if stock_data.empty:
            return [TextContent(type="text", text=f"未找到相关股票")]

        result_list = []
        for _, row in stock_data.iterrows():
            result_list.append({
                "股票代码": row['代码'],
                "股票名称": row['名称'],
                "最新价": row['最新价'],
                "涨跌幅": f"{row['涨跌幅']}%",
                "涨跌额": row['涨跌额'],
                "成交量": row['成交量'],
                "成交额": row['成交额'],
                "最高": row['最高'],
                "最低": row['最低']
            })

        return [TextContent(type="text", text=json.dumps(result_list, ensure_ascii=False, indent=2))]
    except Exception as e:
        return [TextContent(type="text", text=f"批量获取股票行情失败: {str(e)}")]

async def get_index_realtime(index_code: str) -> list[TextContent]:
    try:
        df = ak.stock_zh_index_spot_em()
        index_data = df[df['代码'] == index_code]

        if index_data.empty:
            return [TextContent(type="text", text=f"未找到指数 {index_code}，请检查代码是否正确")]

        row = index_data.iloc[0]
        result = {
            "指数代码": row['代码'],
            "指数名称": row['名称'],
            "最新价": row['最新价'],
            "涨跌幅": f"{row['涨跌幅']}%",
            "涨跌额": row['涨跌额'],
            "成交量": row['成交量'],
            "成交额": row['成交额'],
            "最高": row['最高'],
            "最低": row['最低'],
            "今开": row['今开'],
            "昨收": row['昨收']
        }

        return [TextContent(type="text", text=json.dumps(result, ensure_ascii=False, indent=2))]
    except Exception as e:
        return [TextContent(type="text", text=f"获取指数行情失败: {str(e)}")]

async def get_stock_history(stock_code: str, period: str, start_date: str = None, end_date: str = None) -> list[TextContent]:
    try:
        if period == "daily":
            df = ak.stock_zh_a_hist(symbol=stock_code, period="daily", start_date=start_date, end_date=end_date, adjust="qfq")
        elif period == "weekly":
            df = ak.stock_zh_a_hist(symbol=stock_code, period="weekly", start_date=start_date, end_date=end_date, adjust="qfq")
        elif period == "monthly":
            df = ak.stock_zh_a_hist(symbol=stock_code, period="monthly", start_date=start_date, end_date=end_date, adjust="qfq")
        else:
            return [TextContent(type="text", text="不支持的周期类型，请使用 daily/weekly/monthly")]

        if df.empty:
            return [TextContent(type="text", text=f"未找到股票 {stock_code} 的历史数据")]

        df = df.tail(30)
        result_list = []
        for _, row in df.iterrows():
            result_list.append({
                "日期": row['日期'],
                "开盘": row['开盘'],
                "收盘": row['收盘'],
                "最高": row['最高'],
                "最低": row['最低'],
                "成交量": row['成交量'],
                "成交额": row['成交额'],
                "涨跌幅": f"{row['涨跌幅']}%"
            })

        return [TextContent(type="text", text=json.dumps(result_list, ensure_ascii=False, indent=2))]
    except Exception as e:
        return [TextContent(type="text", text=f"获取历史K线失败: {str(e)}")]

async def search_stock(keyword: str) -> list[TextContent]:
    try:
        df = ak.stock_zh_a_spot_em()
        mask = df['代码'].str.contains(keyword, na=False) | df['名称'].str.contains(keyword, na=False)
        result = df[mask][['代码', '名称', '最新价', '涨跌幅', '成交额']].head(20)

        if result.empty:
            return [TextContent(type="text", text=f"未找到包含 '{keyword}' 的股票")]

        result_list = []
        for _, row in result.iterrows():
            result_list.append({
                "股票代码": row['代码'],
                "股票名称": row['名称'],
                "最新价": row['最新价'],
                "涨跌幅": f"{row['涨跌幅']}%",
                "成交额": row['成交额']
            })

        return [TextContent(type="text", text=json.dumps(result_list, ensure_ascii=False, indent=2))]
    except Exception as e:
        return [TextContent(type="text", text=f"搜索股票失败: {str(e)}")]

async def main():
    async with stdio_server() as (read_stream, write_stream):
        await server.run(read_stream, write_stream, server.create_initialization_options())

if __name__ == "__main__":
    import asyncio
    asyncio.run(main())
