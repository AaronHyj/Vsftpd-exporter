#!/usr/bin/env python3
"""
验证 Grafana 仪表板配置文件的有效性
"""
import json
import sys

def validate_dashboard(filename):
    """验证仪表板配置"""
    print(f"🔍 验证仪表板配置: {filename}")
    
    try:
        with open(filename, 'r', encoding='utf-8') as f:
            dashboard = json.load(f)
        
        # 基本结构检查
        required_fields = ['panels', 'title', 'uid', 'templating']
        missing_fields = [field for field in required_fields if field not in dashboard]
        
        if missing_fields:
            print(f"❌ 缺少必需字段: {', '.join(missing_fields)}")
            return False
        
        # 统计信息
        panels = dashboard.get('panels', [])
        panel_count = len(panels)
        row_count = sum(1 for p in panels if p.get('type') == 'row')
        viz_count = panel_count - row_count
        
        print(f"\n✅ 仪表板配置有效")
        print(f"📊 统计信息:")
        print(f"   - 标题: {dashboard.get('title')}")
        print(f"   - UID: {dashboard.get('uid')}")
        print(f"   - 总面板数: {panel_count}")
        print(f"   - 行数: {row_count}")
        print(f"   - 可视化面板数: {viz_count}")
        print(f"   - 刷新间隔: {dashboard.get('refresh', 'N/A')}")
        print(f"   - 标签: {', '.join(dashboard.get('tags', []))}")
        
        # 变量检查
        variables = dashboard.get('templating', {}).get('list', [])
        print(f"   - 变量数: {len(variables)}")
        for var in variables:
            print(f"     • {var.get('name')}: {var.get('label')}")
        
        # 面板类型统计
        panel_types = {}
        for panel in panels:
            ptype = panel.get('type', 'unknown')
            panel_types[ptype] = panel_types.get(ptype, 0) + 1
        
        print(f"\n📈 面板类型分布:")
        for ptype, count in sorted(panel_types.items()):
            print(f"   - {ptype}: {count}")
        
        # 数据源检查
        datasources = set()
        for panel in panels:
            if 'datasource' in panel and panel['datasource']:
                datasources.add(panel['datasource'])
        
        print(f"\n🔌 使用的数据源:")
        for ds in datasources:
            print(f"   - {ds}")
        
        return True
        
    except json.JSONDecodeError as e:
        print(f"❌ JSON 解析错误: {e}")
        return False
    except FileNotFoundError:
        print(f"❌ 文件不存在: {filename}")
        return False
    except Exception as e:
        print(f"❌ 验证失败: {e}")
        return False

if __name__ == '__main__':
    filename = sys.argv[1] if len(sys.argv) > 1 else 'grafana-dashboard.json'
    success = validate_dashboard(filename)
    sys.exit(0 if success else 1)
