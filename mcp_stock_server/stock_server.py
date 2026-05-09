#!/usr/bin/env python3
import asyncio
import json
from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import Tool, TextContent
import akshare as ak

server = Server("stock-info")

async def get_stock_data(stock_code: str, fetch_func):
    try:
        df = await fetch_func(stock_code)
        if df is None or df.empty:
            return None
        return df
    except Exception as e:
        print(f"Error fetching data: {e}", file=__import__('sys').stderr)
        return None

@server.list_tools()
async def list_tools() -> list[Tool]:
    return [
        Tool(
            name="get_stock_quote",
            description="获取A股股票的实时行情数据",
            inputSchema={
                "type": "object",
                "properties": {
                    "stock_code": {
                        "type": "string",
                        "description": "股票代码，如 '000001'（平安银行）、'600519'（贵州茅台）"
                    }
                },
                "required": ["stock_code"]
            }
        ),
        Tool(
            name="get_index_realtime",
            description="获取大盘指数的实时行情",
            inputSchema={
                "type": "object",
                "properties": {
                    "index_code": {
                        "type": "string",
                        "description": "指数代码，如 '000001'（上证指数）、'399001'（深证成指）"
                    }
                },
                "required": ["index_code"]
            }
        ),
        Tool(
            name="search_stock",
            description="搜索股票",
            inputSchema={
                "type": "object",
                "properties": {
                    "keyword": {
                        "type": "string",
                        "description": "搜索关键词"
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
        elif name == "get_index_realtime":
            return await get_index_realtime(arguments.get("index_code"))
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
            return [TextContent(type="text", text=f"未找到股票 {stock_code}")]

        row = stock_data.iloc[0]
        result = {
            "股票代码": row['代码'],
            "股票名称": row['名称'],
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
        return [TextContent(type="text", text=f"获取失败: {str(e)}")]

async def get_index_realtime(index_code: str) -> list[TextContent]:
    try:
        df = ak.stock_zh_index_spot_em()
        index_data = df[df['代码'] == index_code]

        if index_data.empty:
            return [TextContent(type="text", text=f"未找到指数 {index_code}")]

        row = index_data.iloc[0]
        result = {
            "指数代码": row['代码'],
            "指数名称": row['名称'],
            "最新价": row['最新价'],
            "涨跌幅": f"{row['涨跌幅']}%",
            "涨跌额": row['涨跌额'],
            "成交量": row['成交量'],
            "成交额": row['成交额']
        }

        return [TextContent(type="text", text=json.dumps(result, ensure_ascii=False, indent=2))]
    except Exception as e:
        return [TextContent(type="text", text=f"获取失败: {str(e)}")]

async def search_stock(keyword: str) -> list[TextContent]:
    try:
        df = ak.stock_zh_a_spot_em()
        mask = df['代码'].str.contains(keyword, na=False) | df['名称'].str.contains(keyword, na=False)
        result = df[mask][['代码', '名称', '最新价', '涨跌幅']].head(10)

        if result.empty:
            return [TextContent(type="text", text=f"未找到包含 '{keyword}' 的股票")]

        result_list = []
        for _, row in result.iterrows():
            result_list.append({
                "股票代码": row['代码'],
                "股票名称": row['名称'],
                "最新价": row['最新价'],
                "涨跌幅": f"{row['涨跌幅']}%"
            })

        return [TextContent(type="text", text=json.dumps(result_list, ensure_ascii=False, indent=2))]
    except Exception as e:
        return [TextContent(type="text", text=f"搜索失败: {str(e)}")]

async def main():
    async with stdio_server() as (read_stream, write_stream):
        await server.run(read_stream, write_stream, server.create_initialization_options())

if __name__ == "__main__":
    asyncio.run(main())
